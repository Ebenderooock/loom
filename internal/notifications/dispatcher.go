package notifications

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/ebenderooock/loom/internal/kernel/eventbus"
)

// Topic constants duplicated here to avoid import cycles with downloads/
// imports packages (which already import notifications).
const (
	topicDownloadQueued    = "downloads.queued"
	topicDownloadCompleted = "downloads.completed"
	topicDownloadStalled   = "downloads.stalled"
	topicDownloadFailed    = "downloads.failed"
	topicImportCompleted   = "imports.completed"
	topicImportFailed      = "imports.failed"
)

// topicEventMap maps event bus topics to notification EventTypes.
var topicEventMap = map[string]EventType{
	topicDownloadQueued:    EventOnGrab,
	topicDownloadCompleted: EventOnDownload,
	topicDownloadStalled:   EventOnHealthIssue,
	topicDownloadFailed:    EventOnHealthIssue,
	topicImportCompleted:   EventOnDownload,
	topicImportFailed:      EventOnHealthIssue,
}

const (
	maxRetries    = 3
	retryBaseWait = 2 * time.Second
	workerCount   = 4
)

// dispatchJob is a unit of work sent to the worker pool.
type dispatchJob struct {
	conn    *Connection
	notif   Notification
	attempt int
}

// Dispatcher subscribes to the event bus and fans out notifications to
// all enabled connections that match the event type. Sends run in a
// bounded goroutine pool with retry.
type Dispatcher struct {
	bus    eventbus.Bus
	svc    Service
	logger *slog.Logger

	jobs   chan dispatchJob
	wg     sync.WaitGroup
	cancel context.CancelFunc
	unsubs []func()
	done   chan struct{}
}

// NewDispatcher creates a dispatcher but does not start it.
func NewDispatcher(bus eventbus.Bus, svc Service, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{
		bus:    bus,
		svc:    svc,
		logger: logger.With("component", "notification-dispatcher"),
		jobs:   make(chan dispatchJob, 64),
		done:   make(chan struct{}),
	}
}

// Start subscribes to all relevant event bus topics and launches workers.
func (d *Dispatcher) Start(ctx context.Context) {
	ctx, d.cancel = context.WithCancel(ctx)

	// Launch worker pool.
	for i := 0; i < workerCount; i++ {
		d.wg.Add(1)
		go d.worker(ctx)
	}

	// Subscribe to each domain topic.
	for topic, eventType := range topicEventMap {
		et := eventType // capture
		unsub := d.bus.Subscribe(topic, func(_ context.Context, ev eventbus.Event) error {
			d.handleEvent(et, ev)
			return nil
		})
		d.unsubs = append(d.unsubs, unsub)
	}

	d.logger.Info("dispatcher started", "topics", len(topicEventMap), "workers", workerCount)
}

// Stop unsubscribes from all topics and drains the worker pool.
func (d *Dispatcher) Stop() {
	for _, unsub := range d.unsubs {
		unsub()
	}
	d.unsubs = nil

	if d.cancel != nil {
		d.cancel()
	}
	close(d.done)
	close(d.jobs)
	d.wg.Wait()

	d.logger.Info("dispatcher stopped")
}

// handleEvent is called by the event bus handler. It formats the event
// into a Notification, queries matching connections, and enqueues jobs.
func (d *Dispatcher) handleEvent(eventType EventType, ev eventbus.Event) {
	notif := formatEvent(eventType, ev)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conns, err := d.svc.ListConnections(ctx)
	if err != nil {
		d.logger.Error("failed to list connections for dispatch", "error", err)
		return
	}

	dispatched := 0
	for _, c := range conns {
		if !c.Enabled || !c.SubscribesTo(eventType) {
			continue
		}
		select {
		case d.jobs <- dispatchJob{conn: c, notif: notif, attempt: 0}:
			dispatched++
		default:
			d.logger.Warn("dispatch queue full, dropping notification",
				"connection", c.Name, "event", eventType)
		}
	}

	if dispatched > 0 {
		d.logger.Info("dispatched notifications",
			"event", string(eventType), "connections", dispatched)
	}
}

// worker processes dispatch jobs from the channel.
func (d *Dispatcher) worker(ctx context.Context) {
	defer d.wg.Done()
	for job := range d.jobs {
		d.send(ctx, job)
	}
}

// send attempts to deliver a notification, retrying on failure.
func (d *Dispatcher) send(ctx context.Context, job dispatchJob) {
	conn := job.conn
	sender := senderFor(conn.Type)

	// Apply template override if configured.
	notif := job.notif
	if conn.Settings.TemplateOverride != "" && notif.Data != nil {
		rendered, err := RenderTemplate(conn.Settings.TemplateOverride, notif.EventType, notif.Data)
		if err != nil {
			d.logger.Warn("template render failed, using default",
				"connection", conn.Name, "error", err)
		} else {
			notif.Message = rendered
		}
	}

	sendCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	err := sender.Send(sendCtx, notif, conn.Settings)
	cancel()

	if err != nil {
		if job.attempt < maxRetries {
			wait := retryBaseWait * time.Duration(1<<uint(job.attempt))
			d.logger.Warn("send failed, scheduling retry",
				"connection", conn.Name,
				"attempt", job.attempt+1,
				"wait", wait,
				"error", err,
			)
			time.AfterFunc(wait, func() {
				select {
				case <-d.done:
					d.logger.Warn("retry dropped, dispatcher stopped", "connection", conn.Name)
				case d.jobs <- dispatchJob{conn: conn, notif: job.notif, attempt: job.attempt + 1}:
				default:
					d.logger.Warn("retry queue full, dropping", "connection", conn.Name)
				}
			})
		} else {
			d.logger.Error("send failed permanently",
				"connection", conn.Name,
				"type", conn.Type,
				"event", notif.EventType,
				"error", err,
			)
		}
	} else {
		d.logger.Debug("notification sent",
			"connection", conn.Name,
			"type", conn.Type,
			"event", notif.EventType,
		)
	}

	// Log to history (best-effort).
	errMsg := ""
	success := err == nil
	if err != nil {
		errMsg = err.Error()
	}
	if success || job.attempt >= maxRetries {
		hctx, hcancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer hcancel()
		connID := conn.ID
		if logErr := d.svc.LogHistory(hctx, &connID, string(notif.EventType), notif.Title, notif.Message, success, errMsg); logErr != nil {
			d.logger.Warn("failed to log notification history", "error", logErr)
		}
	}
}

// formatEvent converts a domain event into a Notification with a
// human-readable title and message, plus structured data for templates.
// We avoid type-switching on downloads/imports types to prevent import
// cycles. Instead we use a lightweight field-extraction interface.
func formatEvent(eventType EventType, ev eventbus.Event) Notification {
	data := make(map[string]any)
	var title, message string

	topic := ev.Topic()

	// Extract common fields via optional interfaces so we stay decoupled.
	type titler interface{ GetTitle() string }
	type dataProvider interface{ NotificationData() map[string]any }

	if dp, ok := ev.(dataProvider); ok {
		for k, v := range dp.NotificationData() {
			data[k] = v
		}
	}
	if t, ok := ev.(titler); ok {
		data["title"] = t.GetTitle()
	}

	switch topic {
	case topicDownloadQueued:
		title = "Download Grabbed"
		message = fmt.Sprintf("Queued download: %s", dataStr(data, "title"))

	case topicDownloadCompleted:
		title = "Download Completed"
		message = fmt.Sprintf("Completed: %s", dataStr(data, "title"))

	case topicDownloadStalled:
		title = "Download Stalled"
		reason := dataStr(data, "reason")
		if reason != "" {
			message = fmt.Sprintf("Stalled: %s — %s", dataStr(data, "title"), reason)
		} else {
			message = fmt.Sprintf("Stalled: %s", dataStr(data, "title"))
		}

	case topicDownloadFailed:
		title = "Download Failed"
		message = fmt.Sprintf("Failed: %s", dataStr(data, "error"))

	case topicImportCompleted:
		title = "Import Completed"
		message = fmt.Sprintf("Imported: %s", dataStr(data, "title"))

	case topicImportFailed:
		title = "Import Failed"
		message = fmt.Sprintf("Failed to import %s: %s", dataStr(data, "title"), dataStr(data, "error"))

	default:
		title = "Loom Event"
		message = fmt.Sprintf("Event on topic: %s", topic)
		data["topic"] = topic
	}

	return Notification{
		EventType: eventType,
		Title:     title,
		Message:   message,
		Data:      data,
	}
}

func dataStr(data map[string]any, key string) string {
	if v, ok := data[key].(string); ok && v != "" {
		return v
	}
	return "(unknown)"
}
