package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dop251/goja"
)

const (
	jsMaxCallStack      = 2048             // bound recursion so a script can't overflow the host stack
	jsFetchTimeout      = 30 * time.Second // per-request ceiling (also bounded by the run context)
	jsMaxRequestBody    = 256 * 1024
	jsMaxResponseBody   = 256 * 1024
	jsMaxRequestHeaders = 64
)

// executeJS runs a JavaScript plugin in-process via goja. The VM is created per
// run (goja runtimes are not goroutine-safe and are never shared) and is bounded
// by the run context: a watcher goroutine interrupts the VM on timeout or
// cancellation. Host panics are recovered by the caller (execute); JS exceptions
// and compile errors are recorded as failed runs.
func (r *Runner) executeJS(parent context.Context, p *Plugin, payload Payload) *Run {
	timeout := time.Duration(p.TimeoutSecs) * time.Second
	if timeout <= 0 {
		timeout = defaultTimeout * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	outBuf := &capWriter{limit: maxOutputBytes}
	errBuf := &capWriter{limit: maxOutputBytes}

	vm := goja.New()
	vm.SetMaxCallStackSize(jsMaxCallStack)
	if err := setupJSEnv(vm, ctx, p, payload, outBuf, errBuf); err != nil {
		return r.recordSkip(p, payload.Topic, "failed to set up JS environment: "+err.Error())
	}

	program, err := goja.Compile(p.Name, p.Source, false)
	if err != nil {
		run := &Run{
			PluginID: p.ID, PluginName: p.Name, Topic: payload.Topic,
			Success: false, ExitCode: -1,
			ErrorMsg: "compile error: " + err.Error(), Stderr: err.Error(),
			StartedAt: time.Now().UTC(),
		}
		r.persistRun(run)
		r.logger.Warn("plugin compile failed", "plugin", p.Name, "kind", "js", "error", err)
		return run
	}

	// Interrupt the VM when the run context is done (timeout or cancellation).
	// The done channel stops the watcher once the program returns; a late
	// interrupt is harmless because the VM is never reused.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			vm.Interrupt(ctx.Err())
		case <-done:
		}
	}()

	start := time.Now()
	_, runErr := vm.RunProgram(program)
	dur := time.Since(start)

	run := &Run{
		PluginID:   p.ID,
		PluginName: p.Name,
		Topic:      payload.Topic,
		DurationMs: dur.Milliseconds(),
		Stdout:     outBuf.String(),
		Stderr:     errBuf.String(),
		StartedAt:  start.UTC(),
	}

	var iErr *goja.InterruptedError
	switch {
	case runErr != nil && ctx.Err() != nil:
		// The run context expired/cancelled while the program was executing
		// (e.g. an interrupted loop OR a fetch that observed ctx.Done() and
		// threw). Classify by the context error, not by the error shape, so a
		// blocked fetch reports a timeout rather than a generic JS exception.
		run.Success = false
		run.ExitCode = -1
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			run.ErrorMsg = "timed out after " + timeout.String()
		} else {
			run.ErrorMsg = "cancelled"
		}
	case errors.As(runErr, &iErr):
		run.Success = false
		run.ExitCode = -1
		run.ErrorMsg = "interrupted"
	case runErr != nil:
		run.Success = false
		run.ExitCode = -1
		run.ErrorMsg = runErr.Error()
		if run.Stderr == "" {
			run.Stderr = runErr.Error()
		} else {
			run.Stderr += "\n" + runErr.Error()
		}
	default:
		run.Success = true
		run.ExitCode = 0
	}

	r.persistRun(run)
	if !run.Success {
		r.logger.Warn("plugin run failed", "plugin", p.Name, "event", payload.Event,
			"kind", "js", "error", run.ErrorMsg)
	}
	return run
}

// setupJSEnv installs the plugin runtime API: the event object, an env object,
// console, and fetch.
func setupJSEnv(vm *goja.Runtime, ctx context.Context, p *Plugin, payload Payload, out, errw *capWriter) error {
	// Inject event/env as detached native JS objects (via JSON.parse) so a script
	// cannot mutate host-side Go maps.
	eventJSON, err := json.Marshal(map[string]any{
		"version":   payload.Version,
		"event":     payload.Event,
		"topic":     payload.Topic,
		"title":     payload.Title,
		"data":      payload.Data,
		"timestamp": payload.Timestamp.UTC().Format(time.RFC3339),
	})
	if err != nil {
		return err
	}
	envJSON, err := json.Marshal(p.Env)
	if err != nil {
		return err
	}
	if err := vm.Set("__loom_event_json", string(eventJSON)); err != nil {
		return err
	}
	if err := vm.Set("__loom_env_json", string(envJSON)); err != nil {
		return err
	}
	if _, err := vm.RunString(
		`var event = JSON.parse(__loom_event_json);` +
			`var env = JSON.parse(__loom_env_json || '{}');` +
			`delete __loom_event_json; delete __loom_env_json;`,
	); err != nil {
		return err
	}

	console := vm.NewObject()
	logTo := func(w *capWriter) func(goja.FunctionCall) goja.Value {
		return func(call goja.FunctionCall) goja.Value {
			parts := make([]string, 0, len(call.Arguments))
			for _, a := range call.Arguments {
				parts = append(parts, jsArgToString(a))
			}
			_, _ = w.Write([]byte(strings.Join(parts, " ") + "\n"))
			return goja.Undefined()
		}
	}
	_ = console.Set("log", logTo(out))
	_ = console.Set("info", logTo(out))
	_ = console.Set("debug", logTo(out))
	_ = console.Set("warn", logTo(errw))
	_ = console.Set("error", logTo(errw))
	if err := vm.Set("console", console); err != nil {
		return err
	}

	return vm.Set("fetch", jsFetch(vm, ctx))
}

// jsArgToString renders a console argument: strings verbatim, everything else as
// compact JSON (falling back to the JS string form).
func jsArgToString(v goja.Value) string {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		if v == nil {
			return "undefined"
		}
		return v.String()
	}
	exp := v.Export()
	if s, ok := exp.(string); ok {
		return s
	}
	if b, err := json.Marshal(exp); err == nil {
		return string(b)
	}
	return v.String()
}

// jsFetch implements a minimal, synchronous fetch(url, opts) bound to the run
// context. opts: {method, headers: {}, body}. Returns {status, ok, statusText,
// body, headers}. Only http/https URLs are permitted; bodies and headers are
// size-capped. Errors are thrown as JS exceptions.
func jsFetch(vm *goja.Runtime, ctx context.Context) func(goja.FunctionCall) goja.Value {
	throw := func(msg string) { panic(vm.ToValue(msg)) }
	return func(call goja.FunctionCall) goja.Value {
		urlArg := call.Argument(0)
		if goja.IsUndefined(urlArg) || goja.IsNull(urlArg) {
			throw("fetch: url is required")
		}
		rawURL := urlArg.String()
		u, err := url.Parse(rawURL)
		if err != nil {
			throw("fetch: invalid url: " + err.Error())
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			throw("fetch: only http and https URLs are allowed")
		}

		method := http.MethodGet
		var body string
		headers := map[string]string{}
		if opts := call.Argument(1); !goja.IsUndefined(opts) && !goja.IsNull(opts) {
			if m, ok := opts.Export().(map[string]any); ok {
				if mv, ok := m["method"].(string); ok && strings.TrimSpace(mv) != "" {
					method = strings.ToUpper(mv)
				}
				if bv, ok := m["body"].(string); ok {
					body = bv
				}
				if hv, ok := m["headers"].(map[string]any); ok {
					if len(hv) > jsMaxRequestHeaders {
						throw("fetch: too many request headers")
					}
					for k, val := range hv {
						headers[k] = fmt.Sprintf("%v", val)
					}
				}
			}
		}
		if len(body) > jsMaxRequestBody {
			throw("fetch: request body exceeds limit")
		}

		req, err := http.NewRequestWithContext(ctx, method, rawURL, strings.NewReader(body))
		if err != nil {
			throw("fetch: " + err.Error())
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		client := &http.Client{Timeout: jsFetchTimeout}
		resp, err := client.Do(req)
		if err != nil {
			throw("fetch: " + err.Error())
		}
		defer func() { _ = resp.Body.Close() }()
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, jsMaxResponseBody))

		respHeaders := map[string]string{}
		for k := range resp.Header {
			respHeaders[k] = resp.Header.Get(k)
		}
		result := vm.NewObject()
		_ = result.Set("status", resp.StatusCode)
		_ = result.Set("ok", resp.StatusCode >= 200 && resp.StatusCode < 300)
		_ = result.Set("statusText", resp.Status)
		_ = result.Set("body", string(respBody))
		_ = result.Set("headers", respHeaders)
		return result
	}
}
