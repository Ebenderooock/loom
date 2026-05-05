package safety

import (
	"testing"
)

func TestValidateRelease_Safe(t *testing.T) {
	v := NewReleaseValidator(DefaultConfig())
	r := v.ValidateRelease("Movie.2024.1080p.BluRay.x264", 4500, []string{"movie.mkv", "movie.nfo"})
	if !r.Safe {
		t.Fatalf("expected safe, got reasons: %v", r.Reasons)
	}
}

func TestValidateRelease_DangerousExt(t *testing.T) {
	v := NewReleaseValidator(DefaultConfig())
	r := v.ValidateRelease("Movie.2024", 4500, []string{"movie.mkv", "setup.exe"})
	if r.Safe {
		t.Fatal("expected not safe due to .exe")
	}
	if r.Severity != SeverityBlock {
		t.Fatalf("expected block severity, got %s", r.Severity)
	}
}

func TestValidateRelease_SuspiciousPattern(t *testing.T) {
	v := NewReleaseValidator(DefaultConfig())
	r := v.ValidateRelease("Movie.2024.CRACKED", 4500, nil)
	if r.Safe {
		t.Fatal("expected not safe due to 'crack' pattern")
	}
	if r.Severity != SeverityWarning {
		t.Fatalf("expected warning severity, got %s", r.Severity)
	}
}

func TestValidateRelease_TooSmall(t *testing.T) {
	v := NewReleaseValidator(DefaultConfig())
	r := v.ValidateRelease("Movie.2024", 10, nil)
	if r.Safe {
		t.Fatal("expected not safe due to small size")
	}
}

func TestValidateRelease_TooLarge(t *testing.T) {
	v := NewReleaseValidator(DefaultConfig())
	r := v.ValidateRelease("Movie.2024", 200_000, nil)
	if r.Safe {
		t.Fatal("expected not safe due to large size")
	}
}

func TestValidateRelease_DisabledExtCheck(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BlockDangerousExtensions = false
	v := NewReleaseValidator(cfg)
	r := v.ValidateRelease("Movie.2024", 4500, []string{"setup.exe"})
	if !r.Safe {
		t.Fatalf("expected safe when ext check disabled, got: %v", r.Reasons)
	}
}
