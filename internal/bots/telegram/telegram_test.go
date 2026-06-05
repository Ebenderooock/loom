package telegram

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/bots"
)

type stubHandler struct {
	reply bots.Reply
	mu    sync.Mutex
	got   []bots.Command
}

func (s *stubHandler) Handle(_ context.Context, cmd bots.Command) bots.Reply {
	s.mu.Lock()
	s.got = append(s.got, cmd)
	s.mu.Unlock()
	return s.reply
}

func (s *stubHandler) commands() []bots.Command {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]bots.Command(nil), s.got...)
}

func TestBot_ProcessMessageSendsReply(t *testing.T) {
	h := &stubHandler{reply: bots.Reply{Text: "hello *world*", Buttons: []bots.Button{{Label: "Tap", Data: "req|movie|1"}}}}

	var (
		mu     sync.Mutex
		sent   map[string]any
		served bool
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch {
		case strings.HasSuffix(r.URL.Path, "/getUpdates"):
			mu.Lock()
			first := !served
			served = true
			mu.Unlock()
			if first {
				writeJSON(w, map[string]any{"ok": true, "result": []map[string]any{{
					"update_id": 1,
					"message": map[string]any{
						"text": "/search matrix",
						"from": map[string]any{"id": 99, "username": "alice"},
						"chat": map[string]any{"id": 99},
					},
				}}})
				return
			}
			writeJSON(w, map[string]any{"ok": true, "result": []any{}})
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			var p map[string]any
			_ = json.Unmarshal(body, &p)
			mu.Lock()
			sent = p
			mu.Unlock()
			writeJSON(w, map[string]any{"ok": true, "result": map[string]any{}})
		default:
			writeJSON(w, map[string]any{"ok": true, "result": map[string]any{}})
		}
	}))
	defer srv.Close()

	b := New("TESTTOKEN", h, nil)
	b.apiBase = srv.URL
	b.client = srv.Client()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() { _ = b.Run(ctx); close(done) }()

	deadline := time.After(2 * time.Second)
	for {
		mu.Lock()
		s := sent
		mu.Unlock()
		if s != nil {
			break
		}
		select {
		case <-deadline:
			t.Fatal("no sendMessage observed")
		case <-time.After(20 * time.Millisecond):
		}
	}
	cancel()
	<-done

	cmds := h.commands()
	if len(cmds) == 0 || cmds[0].Text != "/search matrix" || cmds[0].ExternalID != "99" {
		t.Fatalf("unexpected commands: %+v", cmds)
	}
	mu.Lock()
	defer mu.Unlock()
	if sent["text"] != "hello world" { // markers stripped
		t.Fatalf("unexpected sent text: %v", sent["text"])
	}
	if _, ok := sent["reply_markup"]; !ok {
		t.Fatalf("expected reply_markup with buttons")
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
