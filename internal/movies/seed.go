package movies

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/google/uuid"
)

// SeedDefaults seeds default quality definitions and profiles if none exist.
func SeedDefaults(ctx context.Context, svc Service) {
	// Only seed if no quality definitions exist yet
	existing, err := svc.ListQualityDefinitions(ctx)
	if err != nil {
		log.Printf("movies: failed to list quality definitions for seeding: %v", err)
		return
	}
	if len(existing) > 0 {
		return
	}

	log.Println("movies: seeding default quality definitions and profiles")

	type qdef struct {
		name       string
		title      string
		source     string
		resolution string
		modifier   string
		order      int
	}

	defaults := []qdef{
		{"unknown", "Unknown", "Unknown", "Unknown", "", 1},
		{"sdtv", "SDTV", "TV", "480p", "", 2},
		{"webdl-480p", "WEB-DL 480p", "Web", "480p", "", 3},
		{"dvd", "DVD", "DVD", "480p", "", 4},
		{"hdtv-720p", "HDTV 720p", "TV", "720p", "", 5},
		{"webdl-720p", "WEB-DL 720p", "Web", "720p", "", 6},
		{"webrip-720p", "WEBRip 720p", "WebRip", "720p", "", 7},
		{"bluray-720p", "Bluray 720p", "BluRay", "720p", "", 8},
		{"hdtv-1080p", "HDTV 1080p", "TV", "1080p", "", 9},
		{"webdl-1080p", "WEB-DL 1080p", "Web", "1080p", "", 10},
		{"webrip-1080p", "WEBRip 1080p", "WebRip", "1080p", "", 11},
		{"bluray-1080p", "Bluray 1080p", "BluRay", "1080p", "", 12},
		{"bluray-1080p-remux", "Bluray 1080p Remux", "BluRay", "1080p", "REMUX", 13},
		{"hdtv-2160p", "HDTV 2160p", "TV", "2160p", "", 14},
		{"webdl-2160p", "WEB-DL 2160p", "Web", "2160p", "", 15},
		{"webrip-2160p", "WEBRip 2160p", "WebRip", "2160p", "", 16},
		{"bluray-2160p", "Bluray 2160p", "BluRay", "2160p", "", 17},
		{"bluray-2160p-remux", "Bluray 2160p Remux", "BluRay", "2160p", "REMUX", 18},
	}

	defIDs := make(map[string]string) // name → id
	for _, d := range defaults {
		id := uuid.New().String()
		defIDs[d.name] = id
		qd := &QualityDefinition{
			ID:          id,
			Name:        d.name,
			Title:       d.title,
			Source:      d.source,
			Resolution:  d.resolution,
			Modifier:    d.modifier,
			PreferredAt: d.order,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		if err := svc.AddQualityDefinition(ctx, qd); err != nil {
			log.Printf("movies: failed to seed quality definition %q: %v", d.name, err)
			return
		}
	}

	// Seed default quality profiles (like Radarr)
	type profileDef struct {
		name    string
		cutoff  string
		upgrade bool
		items   []string // quality definition names that are allowed
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

	for _, p := range profiles {
		id := uuid.New().String()
		var items []QualityProfileItem
		cutoffID := defIDs[p.cutoff]
		for _, itemName := range p.items {
			items = append(items, QualityProfileItem{
				ID:        defIDs[itemName],
				Name:      itemName,
				Allowed:   true,
				Preferred: defIDs[itemName] == cutoffID,
			})
		}

		qp := &QualityProfile{
			ID:             id,
			Name:           p.name,
			UpgradeAllowed: p.upgrade,
			Cutoff:         cutoffID,
			Language:       "en",
			Items:          items,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		if err := svc.AddQualityProfile(ctx, qp); err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			log.Printf("movies: failed to seed quality profile %q: %v", p.name, err)
			return
		}
	}

	log.Println("movies: seeded default quality definitions and profiles")
}
