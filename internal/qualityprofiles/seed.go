package qualityprofiles

import (
	"context"
	"encoding/json"
	"log"

	"github.com/ebenderooock/loom/internal/movies"
)

// SeedDefaults seeds default quality profiles into the v2 table if none exist.
// It reads quality definitions from the movies service to build proper item references.
func SeedDefaults(ctx context.Context, store *Store, movieSvc movies.Service) {
	existing, err := store.List(ctx)
	if err != nil {
		log.Printf("qualityprofiles: failed to list for seeding: %v", err)
		return
	}
	if len(existing) > 0 {
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

	// Build name → definition lookup.
	defByName := make(map[string]*movies.QualityDefinition, len(defs))
	for i := range defs {
		defByName[defs[i].Name] = defs[i]
	}

	type profileDef struct {
		name    string
		cutoff  string
		upgrade bool
		items   []string
	}

	profiles := []profileDef{
		{
			name:    "Any",
			cutoff:  "webdl-480p",
			upgrade: true,
			items:   []string{"sdtv", "webdl-480p", "dvd", "hdtv-720p", "webdl-720p", "webrip-720p", "bluray-720p", "hdtv-1080p", "webdl-1080p", "webrip-1080p", "bluray-1080p", "bluray-1080p-remux", "hdtv-2160p", "webdl-2160p", "webrip-2160p", "bluray-2160p", "bluray-2160p-remux"},
		},
		{
			name:    "HD-720p/1080p",
			cutoff:  "bluray-1080p",
			upgrade: true,
			items:   []string{"hdtv-720p", "webdl-720p", "webrip-720p", "bluray-720p", "hdtv-1080p", "webdl-1080p", "webrip-1080p", "bluray-1080p"},
		},
		{
			name:    "HD-1080p",
			cutoff:  "bluray-1080p",
			upgrade: true,
			items:   []string{"hdtv-1080p", "webdl-1080p", "webrip-1080p", "bluray-1080p", "bluray-1080p-remux"},
		},
		{
			name:    "Ultra-HD",
			cutoff:  "bluray-2160p",
			upgrade: true,
			items:   []string{"hdtv-2160p", "webdl-2160p", "webrip-2160p", "bluray-2160p", "bluray-2160p-remux"},
		},
		{
			name:    "HD-720p",
			cutoff:  "bluray-720p",
			upgrade: true,
			items:   []string{"hdtv-720p", "webdl-720p", "webrip-720p", "bluray-720p"},
		},
	}

	log.Println("qualityprofiles: seeding default quality profiles (v2)")

	for _, p := range profiles {
		cutoffID := ""
		if d, ok := defByName[p.cutoff]; ok {
			cutoffID = d.ID
		}

		type item struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Preferred bool   `json:"preferred"`
			Allowed   bool   `json:"allowed"`
		}

		var items []item
		for _, name := range p.items {
			d, ok := defByName[name]
			if !ok {
				continue
			}
			items = append(items, item{
				ID:        d.ID,
				Name:      name,
				Preferred: d.ID == cutoffID,
				Allowed:   true,
			})
		}

		itemsJSON, err := json.Marshal(items)
		if err != nil {
			log.Printf("qualityprofiles: failed to marshal items for %q: %v", p.name, err)
			continue
		}

		qp := &QualityProfile{
			Name:           p.name,
			Cutoff:         cutoffID,
			UpgradeAllowed: p.upgrade,
			Items:          string(itemsJSON),
		}
		if err := store.Create(ctx, qp); err != nil {
			log.Printf("qualityprofiles: failed to seed profile %q: %v", p.name, err)
			continue
		}
	}

	log.Println("qualityprofiles: seeded default quality profiles")
}
