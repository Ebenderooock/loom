package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/ebenderooock/loom/internal/kernel/config"
)

func TestRedactsSensitiveKeys(t *testing.T) {
	var buf bytes.Buffer
	l, err := newWith(&buf, config.LogConfig{Level: "info", Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	l.Info("hello", "api_key", "supersecret", "user", "ada")
	out := buf.String()
	if strings.Contains(out, "supersecret") {
		t.Errorf("api_key value leaked: %s", out)
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Errorf("expected redacted marker; got %s", out)
	}
	if !strings.Contains(out, "ada") {
		t.Errorf("non-sensitive value missing: %s", out)
	}
}

func TestParseLevel(t *testing.T) {
	for _, s := range []string{"debug", "info", "warn", "WARNING", "error"} {
		if _, err := parseLevel(s); err != nil {
			t.Errorf("parseLevel(%q) returned error: %v", s, err)
		}
	}
	if _, err := parseLevel("trace"); err == nil {
		t.Error("expected parseLevel(trace) to error")
	}
}

func TestLoggerIncludesTraceContextAttrs(t *testing.T) {
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(prev)
	})

	var buf bytes.Buffer
	l, err := newWith(&buf, config.LogConfig{Level: "info", Format: "json"})
	if err != nil {
		t.Fatal(err)
	}

	ctx, span := otel.Tracer("test").Start(context.Background(), "log-with-trace")
	defer span.End()

	l.InfoContext(ctx, "hello")

	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("unmarshal log record: %v", err)
	}

	if got, want := record["trace_id"], span.SpanContext().TraceID().String(); got != want {
		t.Fatalf("trace_id = %v, want %s", got, want)
	}
	if got, want := record["span_id"], span.SpanContext().SpanID().String(); got != want {
		t.Fatalf("span_id = %v, want %s", got, want)
	}
}

func TestCaptureHandlerIncludesTraceContextAttrs(t *testing.T) {
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(prev)
	})

	rb := NewRingBuffer(10)
	logger := slog.New(NewCaptureHandler(CaptureHandlerConfig{
		Inner:        slog.NewJSONHandler(io.Discard, nil),
		Buffer:       rb,
		CaptureLevel: slog.LevelInfo,
	}))

	ctx, span := otel.Tracer("test").Start(context.Background(), "capture-with-trace")
	defer span.End()

	logger.InfoContext(ctx, "hello")

	entries := rb.Read(1, "")
	if len(entries) != 1 {
		t.Fatalf("expected 1 captured log entry, got %d", len(entries))
	}

	var attrs map[string]any
	if err := json.Unmarshal([]byte(entries[0].Attrs), &attrs); err != nil {
		t.Fatalf("unmarshal captured attrs: %v", err)
	}

	if got, want := attrs["trace_id"], span.SpanContext().TraceID().String(); got != want {
		t.Fatalf("trace_id = %v, want %s", got, want)
	}
	if got, want := attrs["span_id"], span.SpanContext().SpanID().String(); got != want {
		t.Fatalf("span_id = %v, want %s", got, want)
	}
}
