package qualityprofiles

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"reflect"

	"github.com/ebenderooock/loom/internal/movies"
)

type profileDefinition struct {
	name    string
	cutoff  string
	upgrade bool
	items   []string
}

type seededProfileItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Preferred bool   `json:"preferred"`
	Allowed   bool   `json:"allowed"`
}

// SeedDefaults seeds default quality profiles and fixes the ordering of
// unchanged profiles created by the previous seed order.
func SeedDefaults(ctx context.Context, store *Store, movieSvc movies.Service) {
	existing, err := store.List(ctx)
	if err != nil {
		log.Printf("qualityprofiles: failed to list for seeding: %v", err)
		return
	}

	defs, err := movieSvc.ListQualityDefinitions(ctx)
	if err != nil {
		log.Printf("qualityprofiles: failed to list quality definitions for seeding: %v", err)
		return
	}
	if len(defs) == 0 {
		return
	}

	defByName := make(map[string]*movies.QualityDefinition, len(defs))
	for i := range defs {
		defByName[defs[i].Name] = defs[i]
	}

	profiles := defaultProfileDefinitions()
	if len(existing) > 0 {
		migrateUnchangedSeededProfiles(ctx, store, existing, profiles, defByName)
		return
	}

	log.Println("qualityprofiles: seeding default quality profiles (v2)")
	for _, definition := range profiles {
		profile, err := buildDefaultProfile(definition, defByName, false)
		if err != nil {
			log.Printf("qualityprofiles: failed to build profile %q: %v", definition.name, err)
			continue
		}
		if err := store.Create(ctx, profile); err != nil {
			log.Printf("qualityprofiles: failed to seed profile %q: %v", definition.name, err)
		}
	}

	log.Println("qualityprofiles: seeded default quality profiles")
}

// defaultProfileDefinitions are ordered best-to-worst because the search
// engine treats a lower profile tier index as better quality.
func defaultProfileDefinitions() []profileDefinition {
	return []profileDefinition{
		{
			name:    "Any",
			cutoff:  "webdl-480p",
			upgrade: true,
			items:   []string{"bluray-2160p-remux", "bluray-2160p", "webrip-2160p", "webdl-2160p", "hdtv-2160p", "bluray-1080p-remux", "bluray-1080p", "webrip-1080p", "webdl-1080p", "hdtv-1080p", "bluray-720p", "webrip-720p", "webdl-720p", "hdtv-720p", "dvd", "webdl-480p", "sdtv"},
		},
		{
			name:    "HD-720p/1080p",
			cutoff:  "bluray-1080p",
			upgrade: true,
			items:   []string{"bluray-1080p", "webrip-1080p", "webdl-1080p", "hdtv-1080p", "bluray-720p", "webrip-720p", "webdl-720p", "hdtv-720p"},
		},
		{
			name:    "HD-1080p",
			cutoff:  "bluray-1080p",
			upgrade: true,
			items:   []string{"bluray-1080p-remux", "bluray-1080p", "webrip-1080p", "webdl-1080p", "hdtv-1080p"},
		},
		{
			name:    "Ultra-HD",
			cutoff:  "bluray-2160p",
			upgrade: true,
			items:   []string{"bluray-2160p-remux", "bluray-2160p", "webrip-2160p", "webdl-2160p", "hdtv-2160p"},
		},
		{
			name:    "Ultra-HD Remux",
			cutoff:  "bluray-2160p-remux",
			upgrade: true,
			items:   []string{"bluray-2160p-remux", "bluray-2160p", "webrip-2160p", "webdl-2160p", "hdtv-2160p"},
		},
		{
			name:    "HD-720p",
			cutoff:  "bluray-720p",
			upgrade: true,
			items:   []string{"bluray-720p", "webrip-720p", "webdl-720p", "hdtv-720p"},
		},
	}
}

func buildDefaultProfile(
	definition profileDefinition,
	defByName map[string]*movies.QualityDefinition,
	reverse bool,
) (*QualityProfile, error) {
	cutoff, ok := defByName[definition.cutoff]
	if !ok {
		return nil, fmt.Errorf("cutoff definition %q is missing", definition.cutoff)
	}

	names := definition.items
	if reverse {
		names = reversed(names)
	}

	items := make([]seededProfileItem, 0, len(names))
	for _, name := range names {
		def, ok := defByName[name]
		if !ok {
			continue
		}
		items = append(items, seededProfileItem{
			ID:        def.ID,
			Name:      name,
			Preferred: def.ID == cutoff.ID,
			Allowed:   true,
		})
	}

	itemsJSON, err := json.Marshal(items)
	if err != nil {
		return nil, fmt.Errorf("marshal profile items: %w", err)
	}
	return &QualityProfile{
		Name:           definition.name,
		Cutoff:         cutoff.ID,
		UpgradeAllowed: definition.upgrade,
		Items:          string(itemsJSON),
	}, nil
}

func migrateUnchangedSeededProfiles(
	ctx context.Context,
	store *Store,
	existing []QualityProfile,
	definitions []profileDefinition,
	defByName map[string]*movies.QualityDefinition,
) {
	byName := make(map[string]QualityProfile, len(existing))
	for _, profile := range existing {
		byName[profile.Name] = profile
	}

	for _, definition := range definitions {
		current, err := buildDefaultProfile(definition, defByName, false)
		if err != nil {
			log.Printf("qualityprofiles: failed to build profile %q for migration: %v", definition.name, err)
			continue
		}
		previous, err := buildDefaultProfile(definition, defByName, true)
		if err != nil {
			log.Printf("qualityprofiles: failed to build previous profile %q for migration: %v", definition.name, err)
			continue
		}
		profile, ok := byName[definition.name]
		if !ok || profile.Cutoff != previous.Cutoff ||
			profile.UpgradeAllowed != previous.UpgradeAllowed ||
			profile.MinFormatScore != 0 || profile.CutoffFormatScore != 0 ||
			len(profile.FormatItems) != 0 || !sameItems(profile.Items, previous.Items) {
			continue
		}

		profile.Items = current.Items
		if err := store.Update(ctx, &profile); err != nil {
			log.Printf("qualityprofiles: failed to correct seeded profile %q: %v", profile.Name, err)
		}
	}
}

func sameItems(a, b string) bool {
	var left, right []QualityItem
	if json.Unmarshal([]byte(a), &left) != nil || json.Unmarshal([]byte(b), &right) != nil {
		return false
	}
	return reflect.DeepEqual(left, right)
}

func reversed(values []string) []string {
	result := make([]string, len(values))
	for i := range values {
		result[len(values)-1-i] = values[i]
	}
	return result
}
