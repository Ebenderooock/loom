package telemetry

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/loomctl/loom/internal/kernel/config"
)

func TestNewDisabledOTel(t *testing.T) {
	cfg := &config.Config{}
	tel, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = tel.Shutdown(context.Background()) }()
	if tel.OTelEnabled() {
		t.Errorf("OTelEnabled should be false when not configured")
	}
	if tel.Tracer() == nil {
		t.Errorf("Tracer must never be nil")
	}
	if tel.Meter() == nil {
		t.Errorf("Meter must never be nil")
	}
}

func TestMetricsHandlerEmitsPromText(t *testing.T) {
	tel, err := New(context.Background(), &config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tel.Shutdown(context.Background()) }()

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	tel.Handler().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	body := w.Body.String()
	// Process and Go runtime collectors must produce these series.
	for _, want := range []string{"go_goroutines", "process_"} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics body missing %q\n--- body ---\n%s", want, body)
		}
	}
}

func TestInitSetsDefault(t *testing.T) {
	_, err := Init(context.Background(), &config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if Default() == nil {
		t.Errorf("Default should be set after Init")
	}
	if Tracer() == nil {
		t.Errorf("package Tracer() must be non-nil")
	}
	if Meter() == nil {
		t.Errorf("package Meter() must be non-nil")
	}
}

func TestPackageAccessorsBeforeInit(t *testing.T) {
	defaultMu.Lock()
	def = nil
	defaultMu.Unlock()
	if Default() != nil {
		t.Errorf("Default before Init should be nil")
	}
	if Tracer() == nil {
		t.Errorf("Tracer must return a no-op tracer when uninitialized")
	}
	if Meter() == nil {
		t.Errorf("Meter must return a no-op meter when uninitialized")
	}
}
