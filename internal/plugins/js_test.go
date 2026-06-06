package plugins

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func jsRun(t *testing.T, p *Plugin) *Run {
	t.Helper()
	r, _ := newRunner(t, true)
	if len(p.Events) == 0 {
		p.Events = []string{"grab"}
	}
	payload := Payload{
		Version: PayloadVersion, Event: "grab", Topic: topicDownloadQueued,
		Title: "The Matrix", Timestamp: time.Now().UTC(),
		Data: map[string]any{"title": "The Matrix", "year": 1999, "quality": "1080p"},
	}
	return r.executeJS(context.Background(), p, payload)
}

func TestJSConsoleAndEvent(t *testing.T) {
	run := jsRun(t, &Plugin{Name: "console", Source: `
		console.log("title:", event.title);
		console.log("year:", event.data.year);
		console.error("an error line");
	`})
	if !run.Success {
		t.Fatalf("expected success, got error: %s", run.ErrorMsg)
	}
	if !strings.Contains(run.Stdout, "title: The Matrix") {
		t.Errorf("stdout missing title line: %q", run.Stdout)
	}
	if !strings.Contains(run.Stdout, "year: 1999") {
		t.Errorf("stdout missing year line: %q", run.Stdout)
	}
	if !strings.Contains(run.Stderr, "an error line") {
		t.Errorf("stderr missing error line: %q", run.Stderr)
	}
}

func TestJSObjectArgIsJSON(t *testing.T) {
	run := jsRun(t, &Plugin{Name: "json", Source: `console.log(event.data);`})
	if !run.Success {
		t.Fatalf("expected success, got: %s", run.ErrorMsg)
	}
	if !strings.Contains(run.Stdout, `"title":"The Matrix"`) {
		t.Errorf("expected JSON-rendered object, got %q", run.Stdout)
	}
}

func TestJSRuntimeErrorRecorded(t *testing.T) {
	run := jsRun(t, &Plugin{Name: "boom", Source: `throw new Error("kaboom");`})
	if run.Success {
		t.Fatal("expected failure for thrown error")
	}
	if !strings.Contains(run.ErrorMsg, "kaboom") {
		t.Errorf("error message missing thrown text: %q", run.ErrorMsg)
	}
}

func TestJSTimeoutInterrupts(t *testing.T) {
	start := time.Now()
	run := jsRun(t, &Plugin{Name: "loop", TimeoutSecs: 1, Source: `while (true) {}`})
	if run.Success {
		t.Fatal("expected timeout failure")
	}
	if !strings.Contains(run.ErrorMsg, "timed out") {
		t.Errorf("expected timed out error, got %q", run.ErrorMsg)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("timeout took too long: %v", elapsed)
	}
}

func TestJSFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("X-Test") != "1" {
			t.Errorf("missing custom header")
		}
		body, _ := readAll(r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"echo":` + body + `}`))
	}))
	defer srv.Close()

	run := jsRun(t, &Plugin{Name: "fetch", Source: `
		var res = fetch("` + srv.URL + `", { method: "POST", headers: { "X-Test": "1" }, body: "\"hi\"" });
		console.log("status:", res.status, "ok:", res.ok);
		console.log("body:", res.body);
	`})
	if !run.Success {
		t.Fatalf("expected success, got: %s", run.ErrorMsg)
	}
	if !strings.Contains(run.Stdout, "status: 201 ok: true") {
		t.Errorf("unexpected status line: %q", run.Stdout)
	}
	if !strings.Contains(run.Stdout, `"echo":"hi"`) {
		t.Errorf("unexpected body: %q", run.Stdout)
	}
}

func TestJSFetchRejectsNonHTTPScheme(t *testing.T) {
	run := jsRun(t, &Plugin{Name: "badscheme", Source: `fetch("file:///etc/passwd");`})
	if run.Success {
		t.Fatal("expected failure for file:// scheme")
	}
	if !strings.Contains(run.ErrorMsg, "http and https") {
		t.Errorf("expected scheme error, got %q", run.ErrorMsg)
	}
}

func TestJSValidation(t *testing.T) {
	store := NewStore(openTestDB(t))
	ctx := context.Background()
	// Plugin without source rejected.
	if err := store.Create(ctx, &Plugin{Name: "x", Events: []string{"grab"}}); err == nil {
		t.Fatal("expected error for empty source")
	}
	// Unknown event rejected.
	if err := store.Create(ctx, &Plugin{Name: "x", Source: `1;`, Events: []string{"nope"}}); err == nil {
		t.Fatal("expected error for unknown event")
	}
	// Valid plugin round-trips source + env.
	p := &Plugin{Name: "js-ok", Source: `console.log("hi");`,
		Env: map[string]string{"WEBHOOK": "https://x"}, Events: []string{"grab"}}
	if err := store.Create(ctx, p); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := store.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Source != `console.log("hi");` || got.Env["WEBHOOK"] != "https://x" {
		t.Errorf("source/env not persisted: %+v", got)
	}
}

func TestJSEnvExposed(t *testing.T) {
	r, _ := newRunner(t, true)
	p := &Plugin{Name: "env", Source: `console.log("hook:", env.WEBHOOK);`,
		Env: map[string]string{"WEBHOOK": "https://example.com"}, Events: []string{"grab"}}
	payload := Payload{Version: PayloadVersion, Event: "grab", Topic: topicDownloadQueued,
		Title: "x", Timestamp: time.Now().UTC(), Data: map[string]any{}}
	run := r.executeJS(context.Background(), p, payload)
	if !run.Success {
		t.Fatalf("expected success, got: %s", run.ErrorMsg)
	}
	if !strings.Contains(run.Stdout, "hook: https://example.com") {
		t.Errorf("env not exposed to script: %q", run.Stdout)
	}
}

func TestJSCompileErrorRecorded(t *testing.T) {
	run := jsRun(t, &Plugin{Name: "badsyntax", Source: `function {`})
	if run.Success {
		t.Fatal("expected failure for compile error")
	}
	if run.ExitCode != -1 {
		t.Errorf("expected exit code -1, got %d", run.ExitCode)
	}
	if !strings.Contains(run.ErrorMsg, "compile error") {
		t.Errorf("expected compile error message, got %q", run.ErrorMsg)
	}
}

func TestJSFetchTimeoutClassifiedAsTimeout(t *testing.T) {
	// Server sleeps well past the plugin timeout so fetch is still blocked when
	// the run context expires; the run must report a timeout, not a generic
	// JS/network exception.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	start := time.Now()
	run := jsRun(t, &Plugin{Name: "slowfetch", TimeoutSecs: 1,
		Source: `fetch("` + srv.URL + `");`})
	if run.Success {
		t.Fatal("expected timeout failure")
	}
	if !strings.Contains(run.ErrorMsg, "timed out") {
		t.Errorf("expected timed out classification, got %q", run.ErrorMsg)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("run did not fail fast on timeout: %v", elapsed)
	}
}

func readAll(r *http.Request) (string, error) {
	b, err := io.ReadAll(r.Body)
	return string(b), err
}
