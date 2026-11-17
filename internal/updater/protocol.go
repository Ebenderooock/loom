package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ebenderooock/loom/internal/updates"
)

// Identity is the privileged rollback/anti-downgrade anchor, not a release tag.
type Identity struct {
	Version string `json:"version"`
	SHA     string `json:"sha"`
	Ref     string `json:"ref"`
	Channel string `json:"channel"`
}
type state struct {
	Installed Identity `json:"installed"`
	FailedSHA string   `json:"failedSha,omitempty"`
}

// Runtime supplies authenticated readiness and consent, never commands or paths.
type Runtime struct {
	Identity
	Engine   string           `json:"engine"`
	Native   bool             `json:"native"`
	Ready    bool             `json:"ready"`
	Settings updates.Settings `json:"settings"`
}
type publisher interface {
	Discover(context.Context, string) (updates.Candidate, error)
	Download(context.Context, updates.Candidate, string, io.Writer) error
	Ahead(context.Context, string, string) bool
}

func sameIdentity(a, b Identity) bool {
	cmp, err := updates.CompareVersions(a.Version, b.Version)
	return err == nil && cmp == 0 && a.SHA == b.SHA && a.Ref == b.Ref && a.Channel == b.Channel
}
func validConsent(s updates.Settings) bool {
	return (s.Channel == "stable" && !s.PreleaseOptIn) || (s.Channel == "prelease" && s.PreleaseOptIn)
}
func candidateNewer(ctx context.Context, p publisher, old, next Identity) bool {
	if !updates.ValidSource(next.Version, next.Ref, next.SHA, next.Channel) {
		return false
	}
	cmp, err := updates.CompareVersions(next.Version, old.Version)
	if err != nil || cmp < 0 || old.SHA == next.SHA {
		return false
	}
	if cmp > 0 {
		return true
	}
	return old.Channel == "prelease" && next.Channel == "prelease" && old.Ref == next.Ref && p.Ahead(ctx, old.SHA, next.SHA)
}
func validateConfig(path string) error {
	raw, err := os.ReadFile(path) // #nosec G304 -- fixed private snapshot, never a caller path
	if err != nil {
		return err
	}
	var c struct {
		ConfigDir string `yaml:"config_dir"`
		DataDir   string `yaml:"data_dir"`
		HTTP      struct {
			Addr string `yaml:"addr"`
		} `yaml:"http"`
		Storage struct {
			Engine string `yaml:"engine"`
			SQLite struct {
				Path string `yaml:"path"`
			} `yaml:"sqlite"`
		} `yaml:"storage"`
	}
	if yaml.Unmarshal(raw, &c) != nil || c.ConfigDir != ConfigDir || c.DataDir != DataDir || c.HTTP.Addr != "127.0.0.1:1925" || c.Storage.Engine != "sqlite" || c.Storage.SQLite.Path != DBPath {
		return errors.New("updater requires the fixed native SQLite/config/listen paths")
	}
	return nil
}
func readJSON(path string, out any) error {
	raw, err := os.ReadFile(path) // #nosec G304 -- fixed privileged state/journal files
	if err != nil {
		return err
	}
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	if !errors.Is(d.Decode(new(any)), io.EOF) {
		return fmt.Errorf("trailing transaction metadata")
	}
	return nil
}
func syncFile(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0) // #nosec G304 -- only privileged fixed transaction/enrollment files
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return f.Sync()
}
