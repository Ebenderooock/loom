package syncprofiles

import "time"

// SyncProfile controls which indexers a connected app can see.
type SyncProfile struct {
	ID         string                `json:"id"`
	Name       string                `json:"name"`
	AppType    string                `json:"app_type"`
	Enabled    bool                  `json:"enabled"`
	Indexers   []SyncProfileIndexer  `json:"indexers,omitempty"`
	Categories []SyncProfileCategory `json:"categories,omitempty"`
	CreatedAt  time.Time             `json:"created_at"`
	UpdatedAt  time.Time             `json:"updated_at"`
}

// SyncProfileIndexer links an indexer to a sync profile.
type SyncProfileIndexer struct {
	IndexerID string `json:"indexer_id"`
	Enabled   bool   `json:"enabled"`
}

// SyncProfileCategory maps a category within a sync profile.
type SyncProfileCategory struct {
	Category string `json:"category"`
	MappedTo string `json:"mapped_to"`
}

// CreateRequest is the API payload for creating a sync profile.
type CreateRequest struct {
	Name       string                `json:"name"`
	AppType    string                `json:"app_type"`
	Enabled    *bool                 `json:"enabled,omitempty"`
	Indexers   []SyncProfileIndexer  `json:"indexers,omitempty"`
	Categories []SyncProfileCategory `json:"categories,omitempty"`
}

// UpdateRequest is the API payload for updating a sync profile.
type UpdateRequest struct {
	Name       *string               `json:"name,omitempty"`
	AppType    *string               `json:"app_type,omitempty"`
	Enabled    *bool                 `json:"enabled,omitempty"`
	Indexers   []SyncProfileIndexer  `json:"indexers,omitempty"`
	Categories []SyncProfileCategory `json:"categories,omitempty"`
}
