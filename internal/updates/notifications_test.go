package updates

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/kernel/eventbus"
	"github.com/ebenderooock/loom/internal/notifications"
)

type alertBus struct {
	mu     sync.Mutex
	events []notifications.NotificationEvent
	fail   bool
}

func (b *alertBus) Publish(_ context.Context, event eventbus.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.fail {
		return errors.New("bus unavailable")
	}
	b.events = append(b.events, event.(notifications.NotificationEvent))
	return nil
}

func (b *alertBus) Subscribe(string, eventbus.Handler) func() { return func() {} }

func (b *alertBus) snapshot() []notifications.NotificationEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]notifications.NotificationEvent(nil), b.events...)
}

func TestApplicationUpdateAlerts(t *testing.T) {
	s := testService(t)
	bus := &alertBus{}
	ctx := context.Background()
	for _, version := range []string{"1.0.0", "1.0.0", "1.1.0", "1.1.0", "1.0.0"} {
		if err := NewAlerts(s.db, bus).Started(ctx, version); err != nil {
			t.Fatal(err)
		}
	}
	events := bus.snapshot()
	if len(events) != 2 || events[0].Event != notifications.EventOnApplicationUpdate ||
		events[0].Message != "1.0.0 -> 1.1.0" || events[1].Message != "1.1.0 -> 1.0.0" {
		t.Fatalf("unexpected application alerts: %+v", events)
	}
}

func TestAvailableAlertsVerifiedCheckerAndPersistence(t *testing.T) {
	s := testService(t)
	bus := &alertBus{}
	s.ConfigureAlerts(NewAlerts(s.db, bus))
	router := s.Router(func(next http.Handler) http.Handler { return next })

	// No configured trust key: neither a release request nor an alert is allowed.
	s.checker.keys = nil
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/check", nil))
	if w.Code != http.StatusOK || len(bus.snapshot()) != 0 {
		t.Fatalf("unconfigured checker: status=%d alerts=%+v", w.Code, bus.snapshot())
	}

	// The checker fixture uses a freshly generated test-only Ed25519 key.
	s.checker, _ = fixtureChecker(t, "v1.2.3", "stable")
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/check", nil))
			if w.Code != http.StatusOK {
				t.Errorf("check status=%d", w.Code)
			}
		}()
	}
	wg.Wait()
	events := bus.snapshot()
	if len(events) != 1 || events[0].Event != notifications.EventOnUpdateAvailable ||
		!strings.Contains(events[0].Message, "1.0.0 -> v1.2.3") ||
		!strings.Contains(events[0].Message, releaseURL+"/tag/v1.2.3") {
		t.Fatalf("unexpected available alerts: %+v", events)
	}
	restarted := NewService(s.db, "1.0.0")
	restarted.checker, _ = fixtureChecker(t, "v1.2.3", "stable")
	restarted.ConfigureAlerts(NewAlerts(s.db, bus))
	restarted.Router(func(next http.Handler) http.Handler { return next }).ServeHTTP(
		httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/check", nil))
	if len(bus.snapshot()) != 1 {
		t.Fatalf("duplicate alert after restart: %+v", bus.snapshot())
	}
	restarted.checker, _ = fixtureChecker(t, "v1.2.4", "stable")
	restarted.Router(func(next http.Handler) http.Handler { return next }).ServeHTTP(
		httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/check", nil))
	if len(bus.snapshot()) != 2 {
		t.Fatalf("missing new version alert: %+v", bus.snapshot())
	}
}

func TestAlertsRetryPublishFailures(t *testing.T) {
	s := testService(t)
	bus := &alertBus{fail: true}
	alerts := NewAlerts(s.db, bus)
	ctx := context.Background()
	if err := alerts.Started(ctx, "1.0.0"); err != nil {
		t.Fatal(err)
	}

	if err := alerts.Started(ctx, "1.1.0"); err == nil {
		t.Fatal("expected publish failure")
	}
	n := Notice{Verified: true, Available: true, Version: "1.2.0"}
	if err := alerts.Available(ctx, n); err == nil {
		t.Fatal("expected publish failure")
	}
	bus.fail = false
	if err := alerts.Started(ctx, "1.1.0"); err != nil {
		t.Fatal(err)
	}
	if err := alerts.Available(ctx, n); err != nil {
		t.Fatal(err)
	}
	if len(bus.snapshot()) != 2 {
		t.Fatalf("expected alerts after retry: %+v", bus.snapshot())
	}
}

func TestAvailableAlertsRejectUntrustedAndNonNewer(t *testing.T) {
	s := testService(t)
	bus := &alertBus{}
	s.ConfigureAlerts(NewAlerts(s.db, bus))
	s.checker, _ = fixtureChecker(t, "v1.2.3", "stable")
	s.checker.keys, _ = signingKey(t) // A different key cannot verify the fixture.
	router := s.Router(func(next http.Handler) http.Handler { return next })
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/check", nil))
	if len(bus.snapshot()) != 0 {
		t.Fatal("untrusted release produced an alert")
	}
	s.checker, _ = fixtureChecker(t, "v1.0.0", "stable")
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/check", nil))
	if len(bus.snapshot()) != 0 {
		t.Fatal("current release produced an alert")
	}
}

func TestAlertsReachSubscribedWebhook(t *testing.T) {
	s := testService(t)
	_, err := s.db.DB().Exec(`
	CREATE TABLE notification_connections (
		id TEXT PRIMARY KEY, name TEXT NOT NULL, type TEXT NOT NULL, enabled BOOLEAN,
		settings TEXT, on_grab BOOLEAN, on_download BOOLEAN, on_upgrade BOOLEAN,
		on_rename BOOLEAN, on_delete BOOLEAN, on_health_issue BOOLEAN,
		on_application_update BOOLEAN, on_update_available BOOLEAN, on_playback BOOLEAN,
		tags TEXT, created_at TEXT, updated_at TEXT
	);
	CREATE TABLE notification_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT, connection_id TEXT, event_type TEXT,
		title TEXT, message TEXT, success BOOLEAN, error_message TEXT, sent_at TEXT
	);`)
	if err != nil {
		t.Fatal(err)
	}
	type webhookPayload struct {
		Event   notifications.EventType `json:"event"`
		Message string                  `json:"message"`
	}
	received := make(chan webhookPayload, 2)
	webhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var notification webhookPayload
		if err := json.NewDecoder(r.Body).Decode(&notification); err != nil {
			t.Errorf("decode webhook: %v", err)
		} else {
			received <- notification
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer webhook.Close()
	svc := notifications.NewService(s.db.DB())
	conn := &notifications.Connection{
		Name: "updates", Type: notifications.ConnectionTypeWebhook, Enabled: true,
		OnApplicationUpdate: true, OnUpdateAvailable: true,
		Settings: notifications.ConnectionSettings{WebhookURL: webhook.URL},
	}
	if err := svc.CreateConnection(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
	bus := eventbus.NewInProc()
	unsubscribe := notifications.SubscribeToNotifications(bus, svc)
	defer unsubscribe()
	alerts := NewAlerts(s.db, bus)
	if err := alerts.Started(context.Background(), "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := alerts.Started(context.Background(), "1.1.0"); err != nil {
		t.Fatal(err)
	}
	s.checker, _ = fixtureChecker(t, "v1.2.3", "stable")
	s.ConfigureAlerts(alerts)
	s.Router(func(next http.Handler) http.Handler { return next }).ServeHTTP(
		httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/check", nil))
	got := make(map[notifications.EventType]string)
	for range 2 {
		select {
		case notification := <-received:
			got[notification.Event] = notification.Message
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for notifications: %+v", got)
		}
	}
	if got[notifications.EventOnApplicationUpdate] != "1.0.0 -> 1.1.0" ||
		!strings.Contains(got[notifications.EventOnUpdateAvailable], releaseURL+"/tag/v1.2.3") {
		t.Fatalf("unexpected webhook notifications: %+v", got)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		history, err := svc.ListHistory(context.Background(), 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(history) == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for notification history: %+v", history)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
