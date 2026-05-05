package libraries

import "time"

// Library represents a root-folder library that Loom monitors.
type Library struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Path             string    `json:"path"`
	MediaType        string    `json:"media_type"`
	MonitorOnAdd     bool      `json:"monitor_on_add"`
	QualityProfileID string    `json:"quality_profile_id"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`

	// Computed fields (not stored, populated by handlers).
	Accessible   bool      `json:"accessible"`
	DiskSpace    DiskSpace `json:"disk_space"`
	FileCount    int       `json:"file_count"`
	UnmappedCount int      `json:"unmapped_count"`
}

// LibraryFile represents a media file discovered inside a library.
type LibraryFile struct {
	ID          string     `json:"id"`
	LibraryID   string     `json:"library_id"`
	Path        string     `json:"path"`
	SizeBytes   int64      `json:"size_bytes"`
	MediaID     *string    `json:"media_id,omitempty"`
	LastScanned *time.Time `json:"last_scanned,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// DiskSpace holds filesystem usage for a library root path.
type DiskSpace struct {
	TotalBytes int64 `json:"total_bytes"`
	FreeBytes  int64 `json:"free_bytes"`
	UsedBytes  int64 `json:"used_bytes"`
}

// UnmappedFolder represents a subfolder with no matching media entry.
type UnmappedFolder struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// CreateLibraryRequest is the API payload for adding a library.
type CreateLibraryRequest struct {
	Name             string `json:"name"`
	Path             string `json:"path"`
	MediaType        string `json:"media_type"`
	MonitorOnAdd     *bool  `json:"monitor_on_add,omitempty"`
	QualityProfileID string `json:"quality_profile_id,omitempty"`
}

// UpdateLibraryRequest is the API payload for updating a library.
type UpdateLibraryRequest struct {
	Name             *string `json:"name,omitempty"`
	Path             *string `json:"path,omitempty"`
	MediaType        *string `json:"media_type,omitempty"`
	MonitorOnAdd     *bool   `json:"monitor_on_add,omitempty"`
	QualityProfileID *string `json:"quality_profile_id,omitempty"`
}
