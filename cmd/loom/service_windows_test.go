//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows/svc"
)

func TestServiceStopWaitsForCleanup(t *testing.T) {
	for _, command := range []svc.Cmd{svc.Stop, svc.Shutdown} {
		t.Run(fmt.Sprint(command), func(t *testing.T) {
			canceled, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			handler := &loomService{serve: func(ctx context.Context) error {
				<-ctx.Done()
				close(canceled)
				<-release
				return nil
			}}
			requests := make(chan svc.ChangeRequest, 2)
			statuses := make(chan svc.Status, 16)
			result := make(chan uint32, 1)
			go func() {
				_, code := handler.Execute(nil, requests, statuses)
				result <- code
			}()
			if (<-statuses).State != svc.StartPending || (<-statuses).State != svc.Running {
				t.Fatal("expected startup states")
			}
			requests <- svc.ChangeRequest{Cmd: svc.Interrogate}
			if (<-statuses).State != svc.Running {
				t.Fatal("interrogate did not report current state")
			}
			requests <- svc.ChangeRequest{Cmd: command}
			status := <-statuses
			if status.State != svc.StopPending || status.WaitHint == 0 {
				t.Fatal("expected pending shutdown with wait hint")
			}
			select {
			case <-canceled:
			case <-time.After(5 * time.Second):
				t.Fatal("stop did not cancel serve")
			}
			select {
			case <-result:
				t.Fatal("handler exited before cleanup completed")
			default:
			}
			unblock()
			select {
			case code := <-result:
				if code != 0 {
					t.Fatalf("normal shutdown returned exit code %d", code)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("handler did not exit after cleanup")
			}
		})
	}
}

func TestServiceReportsWorkerFailure(t *testing.T) {
	handler := &loomService{serve: func(context.Context) error { return errors.New("startup failed") }}
	statuses := make(chan svc.Status, 4)
	specific, code := handler.Execute(nil, make(chan svc.ChangeRequest), statuses)
	if !specific || code == 0 {
		t.Fatal("worker failure must reach SCM")
	}
}

func TestServiceRejectsRelativePaths(t *testing.T) {
	if err := cmdService([]string{"--config", "loom.yaml", "--log-file", "loom.log"}); err == nil {
		t.Fatal("relative paths must be rejected before SCM access")
	}
}
