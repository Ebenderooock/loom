package imports

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/ebenderooock/loom/internal/downloads"
)

func TestResolveDownloadPathPrefersExistingCandidate(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	existingTitle := "From.S01E01.1080p.WEB-DL"
	existingPath := filepath.Join(tmp, existingTitle)
	if err := os.MkdirAll(existingPath, 0o755); err != nil {
		t.Fatalf("mkdir existing path: %v", err)
	}

	client := &resolvePathFakeClient{
		id: "client-1",
		items: []downloads.Item{{
			ID:          "download-1",
			Title:       existingTitle,
			ContentPath: "/media/downloads/does-not-exist",
			SavePath:    tmp,
		}},
	}

	svc, err := downloads.NewService(downloads.ServiceOptions{
		Repository: resolvePathFakeRepo{},
		Registry:   downloads.NewRegistry(),
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("new downloads service: %v", err)
	}
	if err := svc.Registry().Register(client); err != nil {
		t.Fatalf("register client: %v", err)
	}

	p := &ImportPipeline{downloadSvc: svc}
	got, err := p.resolveDownloadPath(context.Background(), &downloads.DownloadCompletedEvent{
		ClientID:   client.id,
		DownloadID: "download-1",
		Title:      "event-title",
	})
	if err != nil {
		t.Fatalf("resolve download path: %v", err)
	}
	if got != existingPath {
		t.Fatalf("resolved path mismatch: got %q want %q", got, existingPath)
	}
}

type resolvePathFakeClient struct {
	id    string
	items []downloads.Item
}

func (c *resolvePathFakeClient) ID() string                               { return c.id }
func (c *resolvePathFakeClient) Name() string                             { return "fake" }
func (c *resolvePathFakeClient) Kind() downloads.Kind                     { return downloads.KindTransmission }
func (c *resolvePathFakeClient) Protocol() downloads.Protocol             { return downloads.ProtocolTorrent }
func (c *resolvePathFakeClient) Add(context.Context, downloads.AddRequest) (downloads.AddResult, error) {
	return downloads.AddResult{}, nil
}
func (c *resolvePathFakeClient) Status(context.Context, ...string) ([]downloads.Item, error) {
	return c.items, nil
}
func (c *resolvePathFakeClient) Pause(context.Context, ...string) error { return nil }
func (c *resolvePathFakeClient) Resume(context.Context, ...string) error {
	return nil
}
func (c *resolvePathFakeClient) Remove(context.Context, []string, bool) error { return nil }
func (c *resolvePathFakeClient) SetPriority(context.Context, downloads.Priority, ...string) error {
	return nil
}
func (c *resolvePathFakeClient) SetSpeedLimit(context.Context, int64, ...string) error { return nil }
func (c *resolvePathFakeClient) ForceStart(context.Context, ...string) error            { return nil }
func (c *resolvePathFakeClient) Recheck(context.Context, ...string) error               { return nil }
func (c *resolvePathFakeClient) Reannounce(context.Context, ...string) error            { return nil }
func (c *resolvePathFakeClient) Categories(context.Context) ([]downloads.Category, error) {
	return nil, nil
}
func (c *resolvePathFakeClient) FreeSpace(context.Context) (int64, error) { return 0, nil }
func (c *resolvePathFakeClient) Test(context.Context) error                { return nil }

type resolvePathFakeRepo struct{}

func (resolvePathFakeRepo) Create(context.Context, downloads.Definition) (downloads.Definition, error) {
	return downloads.Definition{}, nil
}
func (resolvePathFakeRepo) Get(context.Context, string) (downloads.Definition, error) {
	return downloads.Definition{}, downloads.ErrNotFound
}
func (resolvePathFakeRepo) List(context.Context) ([]downloads.Definition, error)        { return nil, nil }
func (resolvePathFakeRepo) ListEnabled(context.Context) ([]downloads.Definition, error) { return nil, nil }
func (resolvePathFakeRepo) Replace(context.Context, downloads.Definition) (downloads.Definition, error) {
	return downloads.Definition{}, nil
}
func (resolvePathFakeRepo) Patch(context.Context, downloads.Patch) (downloads.Definition, error) {
	return downloads.Definition{}, nil
}
func (resolvePathFakeRepo) Delete(context.Context, string) error { return nil }
func (resolvePathFakeRepo) UpsertHealth(context.Context, downloads.Health) error {
	return nil
}
func (resolvePathFakeRepo) GetHealth(context.Context, string) (downloads.Health, error) {
	return downloads.Health{}, nil
}
func (resolvePathFakeRepo) ListHealth(context.Context) (map[string]downloads.Health, error) {
	return map[string]downloads.Health{}, nil
}
