package libraries

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestDiskSpaceIdentityFailurePreservesCapacity(t *testing.T) {
	path := t.TempDir()
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	space, err := getDiskSpace(path, logger, func(string) (string, error) {
		return "", errors.New("identity unavailable")
	})
	if err != nil {
		t.Fatal(err)
	}
	if space.TotalBytes <= 0 || space.FreeBytes < 0 || space.UsedBytes != space.TotalBytes-space.FreeBytes {
		t.Fatalf("capacity changed: %+v", space)
	}
	if !strings.Contains(logs.String(), "identity unavailable") || !strings.Contains(logs.String(), "volume identity error") {
		t.Fatalf("missing warning: %s", logs.String())
	}
	encoded, err := json.Marshal(space)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "volume_id") {
		t.Fatalf("empty identity should be omitted: %s", encoded)
	}
}
