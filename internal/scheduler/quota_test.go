package scheduler

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestQuotaTryReserveConcurrent(t *testing.T) {
	q := NewQuotaTracker(7)
	var admitted atomic.Int64
	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			if q.TryReserve("provider") {
				admitted.Add(1)
			}
		})
	}
	wg.Wait()
	if admitted.Load() != 7 || q.Usage()["provider"] != 7 {
		t.Fatalf("non-atomic quota admission: admitted=%d usage=%v", admitted.Load(), q.Usage())
	}
	if !q.TryReserve("other") {
		t.Fatal("one exhausted indexer must not block others")
	}
}

func TestQuotaTryReservePrunesAndHonorsNewMax(t *testing.T) {
	q := NewQuotaTracker(1)
	q.records["provider"] = []time.Time{time.Now().Add(-25 * time.Hour)}
	if !q.TryReserve("provider") || q.TryReserve("provider") {
		t.Fatal("expired attempt must free exactly one slot")
	}
	q.SetMax(2)
	if !q.TryReserve("provider") || q.TryReserve("provider") {
		t.Fatal("updated cap not honored")
	}
}
