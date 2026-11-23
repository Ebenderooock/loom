package radarrv3

import (
	"encoding/json"
	"testing"

	"github.com/ebenderooock/loom/internal/libraries"
)

func TestRootFolderJSONUnchanged(t *testing.T) {
	h := &Handler{libIDs: newIDMapper()}
	lib := libraries.Library{ID: "00000001", Path: "/movies", Accessible: true, SizeBytes: 123,
		DiskSpace: libraries.DiskSpace{FreeBytes: 42, VolumeID: "dev:1"}}
	got, err := json.Marshal(h.libraryToRootFolder(lib))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":1,"path":"/movies","accessible":true,"freeSpace":42,"unmappedFolders":[]}`
	if string(got) != want {
		t.Fatalf("got %s; want %s", got, want)
	}
}
