package downloads_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ebenderooock/loom/internal/downloads"
)

// fakeClient is a controllable downloads.DownloadClient for tests.
type fakeClient struct {
	id        string
	name      string
	protocol  downloads.Protocol
	testErr   error
	freeSpace int64
	freeErr   error
	cats      []downloads.Category
	delay     time.Duration

	testCalls atomic.Int64
}

func newFake(id string) *fakeClient {
	return &fakeClient{id: id, name: id, protocol: downloads.ProtocolTorrent, freeSpace: 1024}
}

func (f *fakeClient) ID() string                   { return f.id }
func (f *fakeClient) Name() string                 { return f.name }
func (f *fakeClient) Kind() downloads.Kind         { return downloads.KindNull }
func (f *fakeClient) Protocol() downloads.Protocol { return f.protocol }

func (f *fakeClient) Add(_ context.Context, _ downloads.AddRequest) (downloads.AddResult, error) {
	return downloads.AddResult{ClientID: f.id, ItemID: "item-1"}, nil
}
func (f *fakeClient) Status(_ context.Context, _ ...string) ([]downloads.Item, error) {
	return []downloads.Item{{ID: "item-1", Title: "t", Status: downloads.StatusItemDownloading}}, nil
}
func (f *fakeClient) Pause(_ context.Context, _ ...string) error  { return nil }
func (f *fakeClient) Resume(_ context.Context, _ ...string) error { return nil }
func (f *fakeClient) Remove(_ context.Context, _ []string, _ bool) error {
	return nil
}
func (f *fakeClient) SetPriority(_ context.Context, _ downloads.Priority, _ ...string) error {
	return nil
}
func (f *fakeClient) SetSpeedLimit(_ context.Context, _ int64, _ ...string) error { return nil }
func (f *fakeClient) ForceStart(_ context.Context, _ ...string) error             { return nil }
func (f *fakeClient) Recheck(_ context.Context, _ ...string) error                { return nil }
func (f *fakeClient) Reannounce(_ context.Context, _ ...string) error             { return nil }
func (f *fakeClient) Categories(_ context.Context) ([]downloads.Category, error) {
	return f.cats, nil
}
func (f *fakeClient) FreeSpace(_ context.Context) (int64, error) {
	if f.freeErr != nil {
		return -1, f.freeErr
	}
	return f.freeSpace, nil
}
func (f *fakeClient) Test(ctx context.Context) error {
	f.testCalls.Add(1)
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return f.testErr
}

func TestRegistryRegisterListLen(t *testing.T) {
	t.Parallel()
	r := downloads.NewRegistry()
	if r.Len() != 0 {
		t.Fatalf("empty Len: %d", r.Len())
	}
	if err := r.Register(newFake("a")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := r.Register(newFake("a")); err == nil {
		t.Fatalf("duplicate Register should fail")
	}
	_ = r.Replace(newFake("a"))
	_ = r.Register(newFake("b"))
	list := r.List()
	if len(list) != 2 || list[0].ID() != "a" {
		t.Fatalf("List: %#v", list)
	}
	r.Remove("a")
	if r.Len() != 1 {
		t.Fatalf("after Remove Len=%d", r.Len())
	}
}

func TestRegistryFanOutTest(t *testing.T) {
	t.Parallel()
	r := downloads.NewRegistry()
	good := newFake("good")
	bad := newFake("bad")
	bad.testErr = errors.New("boom")
	_ = r.Register(good)
	_ = r.Register(bad)

	res := r.Test(context.Background(), downloads.FanOutOptions{PerClientTimeout: time.Second})
	if len(res.OK) != 1 || res.OK[0] != "good" {
		t.Fatalf("OK: %#v", res.OK)
	}
	if res.Errors["bad"] == "" {
		t.Fatalf("expected error for bad: %#v", res.Errors)
	}
}

func TestRegistryFanOutFreeSpaceAndStatus(t *testing.T) {
	t.Parallel()
	r := downloads.NewRegistry()
	a := newFake("a")
	a.freeSpace = 100
	b := newFake("b")
	b.freeSpace = 200
	_ = r.Register(a)
	_ = r.Register(b)

	fs := r.FreeSpace(context.Background(), downloads.FanOutOptions{})
	if fs.BytesByClient["a"] != 100 || fs.BytesByClient["b"] != 200 {
		t.Fatalf("FreeSpace: %#v", fs.BytesByClient)
	}

	st := r.Status(context.Background(), nil, downloads.FanOutOptions{})
	if len(st.Items) != 2 {
		t.Fatalf("Status items: %#v", st.Items)
	}
}
