package telemetry

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestNewAppMetrics_RegistersReliabilityMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewAppMetrics(reg)

	m.InternalWriteFailures.WithLabelValues("audit_log").Inc()
	m.OrchestratorCommandDrops.WithLabelValues("workflows.CmdGrabbed").Inc()
	m.OrchestratorCommandBufferDepth.Set(42)
	m.OrchestratorCommandBufferUseRate.Set(0.5)

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	seen := map[string]bool{}
	for _, mf := range families {
		seen[mf.GetName()] = true
	}
	for _, want := range []string{
		"loom_internal_write_failures_total",
		"loom_workflow_orchestrator_command_drops_total",
		"loom_workflow_orchestrator_command_buffer_depth",
		"loom_workflow_orchestrator_command_buffer_utilization_ratio",
	} {
		if !seen[want] {
			t.Fatalf("expected metric %q to be registered", want)
		}
	}
}
