package telemetry

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestHTTPInFlightGauge(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := NewHTTPMetrics(reg)

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})

	handler := metrics.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	go func() {
		handler.ServeHTTP(w, req)
		close(done)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("request did not start")
	}

	inFlight := testutil.ToFloat64(metrics.reqInFlight.WithLabelValues(http.MethodGet, "/test"))
	if inFlight != 1 {
		t.Fatalf("in-flight gauge = %v, want 1", inFlight)
	}

	close(release)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("request did not finish")
	}

	inFlight = testutil.ToFloat64(metrics.reqInFlight.WithLabelValues(http.MethodGet, "/test"))
	if inFlight != 0 {
		t.Fatalf("in-flight gauge = %v, want 0", inFlight)
	}
}
