package importlists

import (
	"context"
	"strings"
	"testing"

	"github.com/ebenderooock/loom/internal/libraries"
)

type libraryServiceStub struct {
	libraries []libraries.Library
}

func (s libraryServiceStub) List(context.Context) ([]libraries.Library, error) {
	return s.libraries, nil
}

func TestResolveLibraryIDWithoutSelection(t *testing.T) {
	tests := []struct {
		name      string
		libraries []libraries.Library
		mediaType string
		wantID    string
		wantError string
	}{
		{
			name: "uses the sole library for the requested media type",
			libraries: []libraries.Library{
				{ID: "movies", MediaType: string(MediaTypeMovie)},
				{ID: "series", MediaType: string(MediaTypeSeries)},
			},
			mediaType: string(MediaTypeMovie),
			wantID:    "movies",
		},
		{
			name: "uses the sole series library when adding a show",
			libraries: []libraries.Library{
				{ID: "movies", MediaType: string(MediaTypeMovie)},
				{ID: "series", MediaType: string(MediaTypeSeries)},
			},
			mediaType: string(MediaTypeSeries),
			wantID:    "series",
		},
		{
			name:      "errors when no matching library exists",
			libraries: []libraries.Library{{ID: "series", MediaType: string(MediaTypeSeries)}},
			mediaType: string(MediaTypeMovie),
			wantError: "no movie library configured",
		},
		{
			name: "errors when multiple matching libraries exist",
			libraries: []libraries.Library{
				{ID: "movies-a", MediaType: string(MediaTypeMovie)},
				{ID: "movies-b", MediaType: string(MediaTypeMovie)},
			},
			mediaType: string(MediaTypeMovie),
			wantError: "multiple movie libraries configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewSyncManager(nil, nil)
			manager.SetLibraryService(libraryServiceStub{libraries: tt.libraries})

			got, err := manager.resolveLibraryID(context.Background(), "", tt.mediaType)
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("expected error containing %q, got %v", tt.wantError, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveLibraryID returned error: %v", err)
			}
			if got != tt.wantID {
				t.Fatalf("resolveLibraryID = %q, want %q", got, tt.wantID)
			}
		})
	}
}
