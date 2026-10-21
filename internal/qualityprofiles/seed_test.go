package qualityprofiles

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/ebenderooock/loom/internal/movies"
)

func TestDefaultProfileDefinitionsAreBestToWorst(t *testing.T) {
	definitions := make(map[string]*movies.QualityDefinition)
	for _, profile := range defaultProfileDefinitions() {
		for _, name := range profile.items {
			definitions[name] = &movies.QualityDefinition{ID: name, Name: name}
		}
	}

	var anyProfile profileDefinition
	for _, profile := range defaultProfileDefinitions() {
		if profile.name == "Any" {
			anyProfile = profile
			break
		}
	}
	if anyProfile.name == "" {
		t.Fatal("Any profile definition not found")
	}

	profile, err := buildDefaultProfile(anyProfile, definitions, false)
	if err != nil {
		t.Fatalf("build default profile: %v", err)
	}
	items, err := profile.ParseItems()
	if err != nil {
		t.Fatalf("parse default profile items: %v", err)
	}
	if len(items) == 0 || items[0].Name != "bluray-2160p-remux" {
		t.Fatalf("best quality should be first, got %#v", items)
	}
	if items[len(items)-1].Name != "sdtv" {
		t.Fatalf("worst quality should be last, got %q", items[len(items)-1].Name)
	}
}

func TestMigrateUnchangedSeededProfileOnly(t *testing.T) {
	ctx := context.Background()
	store := newSeedTestStore(t)
	definitions := make(map[string]*movies.QualityDefinition)
	for _, profile := range defaultProfileDefinitions() {
		for _, name := range profile.items {
			definitions[name] = &movies.QualityDefinition{ID: name, Name: name}
		}
	}

	var anyProfile profileDefinition
	for _, profile := range defaultProfileDefinitions() {
		if profile.name == "Any" {
			anyProfile = profile
			break
		}
	}
	oldProfile, err := buildDefaultProfile(anyProfile, definitions, true)
	if err != nil {
		t.Fatalf("build previous default profile: %v", err)
	}
	oldProfile.ID = "default-profile"
	if err := store.Create(ctx, oldProfile); err != nil {
		t.Fatalf("create previous default profile: %v", err)
	}

	migrateUnchangedSeededProfiles(ctx, store, []QualityProfile{*oldProfile}, defaultProfileDefinitions(), definitions)
	got, err := store.Get(ctx, oldProfile.ID)
	if err != nil {
		t.Fatalf("get migrated profile: %v", err)
	}
	want, err := buildDefaultProfile(anyProfile, definitions, false)
	if err != nil {
		t.Fatalf("build current default profile: %v", err)
	}
	if !sameItems(got.Items, want.Items) {
		t.Fatalf("unchanged built-in profile was not migrated")
	}

	customized, err := buildDefaultProfile(anyProfile, definitions, true)
	if err != nil {
		t.Fatalf("build customized profile: %v", err)
	}
	customized.ID = "custom-profile"
	items, err := customized.ParseItems()
	if err != nil {
		t.Fatalf("parse profile items: %v", err)
	}
	items[0].Preferred = !items[0].Preferred
	customizedJSON, err := json.Marshal(items)
	if err != nil {
		t.Fatalf("marshal customized profile: %v", err)
	}
	customized.Items = string(customizedJSON)
	if err := store.Create(ctx, customized); err != nil {
		t.Fatalf("create customized profile: %v", err)
	}

	migrateUnchangedSeededProfiles(ctx, store, []QualityProfile{*customized}, defaultProfileDefinitions(), definitions)
	got, err = store.Get(ctx, customized.ID)
	if err != nil {
		t.Fatalf("get customized profile: %v", err)
	}
	if !sameItems(got.Items, customized.Items) {
		t.Fatal("customized profile was changed by migration")
	}
}

func newSeedTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`
		CREATE TABLE quality_profiles_v2 (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			cutoff TEXT NOT NULL,
			min_format_score INTEGER NOT NULL,
			cutoff_format_score INTEGER NOT NULL,
			upgrade_allowed INTEGER NOT NULL,
			items TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);
		CREATE TABLE quality_profile_format_items (
			profile_id TEXT NOT NULL,
			format_id TEXT NOT NULL,
			score INTEGER NOT NULL,
			PRIMARY KEY (profile_id, format_id)
		);`)
	if err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	return NewStore(db)
}
