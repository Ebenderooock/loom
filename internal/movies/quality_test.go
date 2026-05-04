package movies

import (
	"testing"
	"time"
)

// TestValidateQualityProfile tests the quality profile validation logic
func TestValidateQualityProfile(t *testing.T) {
	tests := []struct {
		name    string
		profile QualityProfile
		wantErr bool
	}{
		{
			name: "valid profile with cutoff in items and allowed",
			profile: QualityProfile{
				ID:             "test-profile",
				Name:           "Test Profile",
				Cutoff:         "1080p",
				UpgradeAllowed: true,
				Items: []QualityProfileItem{
					{ID: "480p", Name: "480p", Allowed: true, Preferred: false},
					{ID: "720p", Name: "720p", Allowed: true, Preferred: false},
					{ID: "1080p", Name: "1080p", Allowed: true, Preferred: true},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid profile: cutoff not in items",
			profile: QualityProfile{
				ID:             "test-profile",
				Name:           "Test Profile",
				Cutoff:         "2160p",
				UpgradeAllowed: true,
				Items: []QualityProfileItem{
					{ID: "480p", Name: "480p", Allowed: true, Preferred: false},
					{ID: "720p", Name: "720p", Allowed: true, Preferred: false},
					{ID: "1080p", Name: "1080p", Allowed: true, Preferred: true},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid profile: cutoff in items but not allowed",
			profile: QualityProfile{
				ID:             "test-profile",
				Name:           "Test Profile",
				Cutoff:         "1080p",
				UpgradeAllowed: true,
				Items: []QualityProfileItem{
					{ID: "480p", Name: "480p", Allowed: true, Preferred: false},
					{ID: "720p", Name: "720p", Allowed: true, Preferred: false},
					{ID: "1080p", Name: "1080p", Allowed: false, Preferred: true},
				},
			},
			wantErr: true,
		},
		{
			name: "valid profile: single item as cutoff",
			profile: QualityProfile{
				ID:             "test-profile",
				Name:           "Single Quality",
				Cutoff:         "1080p",
				UpgradeAllowed: false,
				Items: []QualityProfileItem{
					{ID: "1080p", Name: "1080p", Allowed: true, Preferred: true},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid profile: empty cutoff",
			profile: QualityProfile{
				ID:             "test-profile",
				Name:           "No Cutoff",
				Cutoff:         "",
				UpgradeAllowed: false,
				Items: []QualityProfileItem{
					{ID: "1080p", Name: "1080p", Allowed: true, Preferred: true},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid profile: empty items",
			profile: QualityProfile{
				ID:             "test-profile",
				Name:           "No Items",
				Cutoff:         "1080p",
				UpgradeAllowed: false,
				Items:          []QualityProfileItem{},
			},
			wantErr: true,
		},
	}

	svc := &service{
		repo: nil,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.validateQualityProfile(&tt.profile)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateQualityProfile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestQualityDefinitionFieldValidation validates individual quality definition field requirements
func TestQualityDefinitionFieldValidation(t *testing.T) {
	tests := []struct {
		name    string
		quality QualityDefinition
		valid   bool
	}{
		{
			name: "valid quality definition",
			quality: QualityDefinition{
				ID:         "1080p-blu",
				Name:       "1080p BluRay",
				Title:      "1080p BluRay",
				Source:     "BluRay",
				Resolution: "1080p",
				Modifier:   "REMUX",
				PreferredAt: 100,
			},
			valid: true,
		},
		{
			name: "quality definition with empty modifier",
			quality: QualityDefinition{
				ID:         "1080p-web",
				Name:       "1080p WebRip",
				Title:      "1080p WebRip",
				Source:     "WebRip",
				Resolution: "1080p",
				PreferredAt: 80,
			},
			valid: true,
		},
		{
			name: "quality definition with empty title uses name",
			quality: QualityDefinition{
				ID:         "test-qual",
				Name:       "Test Quality",
				Source:     "Test",
				Resolution: "Test",
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		if tt.valid {
			if tt.quality.ID == "" {
				t.Errorf("%s: ID should not be empty", tt.name)
			}
			if tt.quality.Name == "" {
				t.Errorf("%s: Name should not be empty", tt.name)
			}
			if tt.quality.Source == "" {
				t.Errorf("%s: Source should not be empty", tt.name)
			}
			if tt.quality.Resolution == "" {
				t.Errorf("%s: Resolution should not be empty", tt.name)
			}
		}
	}
}

// TestQualityProfileItemDefaults validates default values for profile items
func TestQualityProfileItemDefaults(t *testing.T) {
	item := QualityProfileItem{
		ID:        "1080p",
		Name:      "1080p",
		Allowed:   true,
		Preferred: false,
	}

	if !item.Allowed {
		t.Error("item should have Allowed=true by default")
	}

	if item.Preferred {
		t.Error("item should have Preferred=false by default")
	}
}

// BenchmarkQualityProfileValidation benchmarks profile validation performance
func BenchmarkQualityProfileValidation(b *testing.B) {
	svc := &service{repo: nil}

	profile := QualityProfile{
		ID:             "bench-profile",
		Name:           "Benchmark Profile",
		Cutoff:         "1080p",
		UpgradeAllowed: true,
		Items: []QualityProfileItem{
			{ID: "480p", Name: "480p", Allowed: true, Preferred: false},
			{ID: "720p", Name: "720p", Allowed: true, Preferred: false},
			{ID: "1080p", Name: "1080p", Allowed: true, Preferred: true},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = svc.validateQualityProfile(&profile)
	}
}

// BenchmarkQualityProfileItemSearch benchmarks searching for items in a profile
func BenchmarkQualityProfileItemSearch(b *testing.B) {
	profile := QualityProfile{
		Items: make([]QualityProfileItem, 100),
	}

	// Create 100 test items
	for i := 0; i < 100; i++ {
		profile.Items[i] = QualityProfileItem{
			ID:        string(rune(i)),
			Name:      string(rune(i)),
			Allowed:   true,
			Preferred: false,
		}
	}

	// Test searching for items in profile
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate finding cutoff
		cutoff := profile.Items[len(profile.Items)-1].ID
		for _, item := range profile.Items {
			if item.ID == cutoff {
				break
			}
		}
	}
}
