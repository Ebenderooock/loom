package series

import (
	"context"
	"testing"
)

func TestPostRefreshHook_WiringViaOption(t *testing.T) {
	called := false
	var gotID string
	hook := func(_ context.Context, seriesID string) {
		called = true
		gotID = seriesID
	}

	svc := NewService(newMockRepo(), "", WithPostRefreshHook(hook)).(*service)
	if svc.postRefreshHook == nil {
		t.Fatalf("WithPostRefreshHook did not set the hook")
	}
	svc.postRefreshHook(context.Background(), "abc")
	if !called || gotID != "abc" {
		t.Fatalf("hook not invoked correctly: called=%v id=%q", called, gotID)
	}
}

func TestPostRefreshHook_WiringViaSetter(t *testing.T) {
	called := false
	svc := NewService(newMockRepo(), "").(*service)
	if svc.postRefreshHook != nil {
		t.Fatalf("expected no hook before setter")
	}
	svc.SetPostRefreshHook(func(_ context.Context, _ string) { called = true })
	if svc.postRefreshHook == nil {
		t.Fatalf("SetPostRefreshHook did not set the hook")
	}
	svc.postRefreshHook(context.Background(), "x")
	if !called {
		t.Fatalf("hook set via setter was not invoked")
	}
}
