package notifications

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUpdateAvailableConnectionAPI(t *testing.T) {
	db := openNotificationsTestDB(t)
	router := Router(NewService(db))

	request := func(method, path, body string, status int) Connection {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		if w.Code != status {
			t.Fatalf("%s %s: got status %d, want %d: %s", method, path, w.Code, status, w.Body.String())
		}
		var c Connection
		if err := json.Unmarshal(w.Body.Bytes(), &c); err != nil {
			t.Fatal(err)
		}
		return c
	}

	c := request(http.MethodPost, "/", `{"name":"default","type":"webhook","on_application_update":true}`, http.StatusCreated)
	if c.OnUpdateAvailable || !c.OnApplicationUpdate {
		t.Fatalf("unexpected defaults: %+v", c)
	}
	c = request(http.MethodPut, "/"+c.ID, `{"on_update_available":true}`, http.StatusOK)
	if !c.OnUpdateAvailable || !c.OnApplicationUpdate {
		t.Fatalf("enabling changed unrelated toggle: %+v", c)
	}
	c = request(http.MethodPut, "/"+c.ID, `{"name":"renamed"}`, http.StatusOK)
	if !c.OnUpdateAvailable {
		t.Fatal("omitting toggle should preserve it")
	}
	c = request(http.MethodGet, "/"+c.ID, "", http.StatusOK)
	if !c.OnUpdateAvailable || !c.OnApplicationUpdate {
		t.Fatalf("toggles not persisted: %+v", c)
	}
	c = request(http.MethodPut, "/"+c.ID, `{"on_update_available":false}`, http.StatusOK)
	if c.OnUpdateAvailable || !c.OnApplicationUpdate {
		t.Fatalf("disabling changed unrelated toggle: %+v", c)
	}
	c = request(http.MethodPost, "/", `{"name":"opt-in","type":"webhook","on_update_available":true}`, http.StatusCreated)
	c = request(http.MethodGet, "/"+c.ID, "", http.StatusOK)
	if !c.OnUpdateAvailable || c.OnApplicationUpdate {
		t.Fatalf("create opt-in not persisted independently: %+v", c)
	}
}

func TestSendUpdateAvailableFiltersConnections(t *testing.T) {
	db := openNotificationsTestDB(t)
	svc := NewService(db)
	ctx := context.Background()
	received := make(chan Notification, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var n Notification
		if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
			t.Errorf("decode notification: %v", err)
		} else {
			received <- n
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	for _, c := range []*Connection{
		{ID: "available", Name: "Available", Enabled: true, OnUpdateAvailable: true},
		{ID: "updated", Name: "Updated", Enabled: true, OnApplicationUpdate: true},
		{ID: "disabled", Name: "Disabled", Enabled: false, OnUpdateAvailable: true},
		{ID: "default", Name: "Default", Enabled: true},
	} {
		c.Type = ConnectionTypeWebhook
		c.Settings.WebhookURL = server.URL
		if err := svc.CreateConnection(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	for _, event := range []EventType{EventOnUpdateAvailable, EventOnApplicationUpdate} {
		if err := svc.Send(ctx, event, "Release", "Message", nil); err != nil {
			t.Fatal(err)
		}
	}
	history, err := svc.ListHistory(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, h := range history {
		if h.ConnectionID == nil || !h.Success {
			t.Fatalf("unexpected history: %+v", h)
		}
		got[*h.ConnectionID] = h.EventType
	}
	if len(history) != 2 || got["available"] != string(EventOnUpdateAvailable) || got["updated"] != string(EventOnApplicationUpdate) {
		t.Fatalf("unexpected deliveries: %+v", history)
	}
	if len(received) != 2 {
		t.Fatalf("got %d webhook requests, want 2", len(received))
	}
}

func TestUpdateAvailableTemplate(t *testing.T) {
	got, err := RenderTemplate("", EventOnUpdateAvailable, map[string]any{"title": "v1.2.3"})
	if err != nil || got != "Update available: v1.2.3" {
		t.Fatalf("RenderTemplate = %q, %v", got, err)
	}
}
