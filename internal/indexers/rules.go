package indexers

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// IndexerRule maps an indexer to specific media type / category / tag filters.
type IndexerRule struct {
	ID             string   `json:"id"`
	IndexerID      string   `json:"indexer_id"`
	MediaType      string   `json:"media_type,omitempty"`
	CategoryFilter []int    `json:"category_filter"`
	TagFilter      []string `json:"tag_filter"`
	Priority       int      `json:"priority"`
	Enabled        bool     `json:"enabled"`
	CreatedAt      string   `json:"created_at,omitempty"`
}

// RuleStore manages indexer rules.
type RuleStore struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewRuleStore creates a RuleStore.
func NewRuleStore(db *sql.DB, logger *slog.Logger) *RuleStore {
	return &RuleStore{db: db, logger: logger}
}

// List returns all indexer rules.
func (rs *RuleStore) List(ctx context.Context) ([]IndexerRule, error) {
	rows, err := rs.db.QueryContext(ctx,
		`SELECT id, indexer_id, media_type, category_filter, tag_filter, priority, enabled, created_at
		 FROM indexer_rules
		 ORDER BY priority DESC, created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []IndexerRule
	for rows.Next() {
		var r IndexerRule
		var mediaType sql.NullString
		var catJSON, tagJSON string
		if err := rows.Scan(&r.ID, &r.IndexerID, &mediaType, &catJSON, &tagJSON, &r.Priority, &r.Enabled, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.MediaType = mediaType.String
		_ = json.Unmarshal([]byte(catJSON), &r.CategoryFilter)
		_ = json.Unmarshal([]byte(tagJSON), &r.TagFilter)
		if r.CategoryFilter == nil {
			r.CategoryFilter = []int{}
		}
		if r.TagFilter == nil {
			r.TagFilter = []string{}
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// Create adds a new indexer rule.
func (rs *RuleStore) Create(ctx context.Context, rule *IndexerRule) error {
	if rule.ID == "" {
		rule.ID = uuid.New().String()
	}
	if rule.CreatedAt == "" {
		rule.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	catJSON, _ := json.Marshal(rule.CategoryFilter)
	tagJSON, _ := json.Marshal(rule.TagFilter)

	var mediaType *string
	if rule.MediaType != "" {
		mediaType = &rule.MediaType
	}

	_, err := rs.db.ExecContext(ctx,
		`INSERT INTO indexer_rules (id, indexer_id, media_type, category_filter, tag_filter, priority, enabled, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		rule.ID, rule.IndexerID, mediaType, string(catJSON), string(tagJSON),
		rule.Priority, rule.Enabled, rule.CreatedAt,
	)
	return err
}

// Delete removes an indexer rule.
func (rs *RuleStore) Delete(ctx context.Context, id string) error {
	_, err := rs.db.ExecContext(ctx, `DELETE FROM indexer_rules WHERE id = ?`, id)
	return err
}

// FilterIndexers returns indexer IDs that are allowed for the given media
// type and tags based on configured rules. If no rules exist, all
// indexers are allowed.
func (rs *RuleStore) FilterIndexers(ctx context.Context, indexerIDs []string, mediaType string, tags []string) ([]string, error) {
	rules, err := rs.List(ctx)
	if err != nil {
		return indexerIDs, nil // fail open
	}

	if len(rules) == 0 {
		return indexerIDs, nil
	}

	// Build set of explicitly allowed indexer IDs
	allowed := make(map[string]bool)
	hasRulesForType := false

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		// Skip if rule is for a different media type
		if rule.MediaType != "" && rule.MediaType != mediaType {
			continue
		}
		hasRulesForType = true

		// Check tag filter
		if len(rule.TagFilter) > 0 {
			if !hasOverlap(tags, rule.TagFilter) {
				continue
			}
		}

		allowed[rule.IndexerID] = true
	}

	// If no rules target this media type, all indexers are valid
	if !hasRulesForType {
		return indexerIDs, nil
	}

	var filtered []string
	for _, id := range indexerIDs {
		if allowed[id] {
			filtered = append(filtered, id)
		}
	}

	if len(filtered) == 0 {
		return indexerIDs, nil // fail open
	}
	return filtered, nil
}

func hasOverlap(a, b []string) bool {
	set := make(map[string]bool, len(b))
	for _, s := range b {
		set[s] = true
	}
	for _, s := range a {
		if set[s] {
			return true
		}
	}
	return false
}
