package notifications

import (
	"context"
	"log"

	"github.com/loomctl/loom/internal/kernel/eventbus"
)

// NotificationEvent is an event bus event that triggers notifications.
type NotificationEvent struct {
	Event     EventType      `json:"event"`
	Title     string         `json:"title"`
	Message   string         `json:"message"`
	Data      map[string]any `json:"data,omitempty"`
}

// Topic implements eventbus.Event.
func (e NotificationEvent) Topic() string {
	return "notification"
}

// Publisher wraps the event bus to provide a simple Publish method for
// notification events. It is safe for concurrent use.
type Publisher struct {
	bus eventbus.Bus
}

// NewPublisher creates a Publisher backed by the given event bus.
func NewPublisher(bus eventbus.Bus) *Publisher {
	return &Publisher{bus: bus}
}

// Publish sends a notification event to the bus. It is non-blocking for
// the caller in that failures are logged, not returned.
func (p *Publisher) Publish(ctx context.Context, event EventType, title, message string, data map[string]any) {
	ev := NotificationEvent{
		Event:   event,
		Title:   title,
		Message: message,
		Data:    data,
	}
	if err := p.bus.Publish(ctx, ev); err != nil {
		log.Printf("notification publish error: %v", err)
	}
}

// SubscribeToNotifications connects the notification service to the event
// bus so that published NotificationEvents fan out to all matching channels.
// Returns an unsubscribe function.
func SubscribeToNotifications(bus eventbus.Bus, svc Service) func() {
	return bus.Subscribe("notification", func(ctx context.Context, ev eventbus.Event) error {
		ne, ok := ev.(NotificationEvent)
		if !ok {
			return nil
		}
		// Send is already goroutine-safe and logs its own errors.
		go func() {
			if err := svc.Send(context.Background(), ne.Event, ne.Title, ne.Message, ne.Data); err != nil {
				log.Printf("notification send via bus failed: %v", err)
			}
		}()
		return nil
	})
}
