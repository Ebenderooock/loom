package updates

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

const repository = "Ebenderooock/loom"

// trusted-keys.json is compiled in, never loaded from writable runtime config.
// Only public keys belong here. Until the isolated publisher provisions one,
// the empty trust root intentionally prevents verified update notices.
//
//go:embed trusted-keys.json
var trustedKeysJSON []byte

// Artifact matches the publisher's platform/archive/digest contract.
type Artifact struct {
	Platform string `json:"platform"`
	Name     string `json:"name"`
	SHA256   string `json:"sha256"`
}

// Manifest is the signed v1 native update metadata contract.
type Manifest struct {
	SchemaVersion int        `json:"schema_version"`
	Channel       string     `json:"channel"`
	Version       string     `json:"version"`
	SourceRef     string     `json:"source_ref"`
	SourceSHA     string     `json:"source_sha"`
	Assets        []Artifact `json:"assets"`
}

func embeddedKeys() map[string]ed25519.PublicKey {
	var encoded map[string]string
	keys := map[string]ed25519.PublicKey{}
	if json.Unmarshal(trustedKeysJSON, &encoded) != nil {
		return keys
	}

	for id, value := range encoded {
		key, err := base64.StdEncoding.DecodeString(value)
		if err == nil && len(key) == ed25519.PublicKeySize {
			keys[id] = ed25519.PublicKey(key)
		}
	}
	return keys
}

// TrustFingerprint binds an enrollment to the exact compiled public trust root.
func TrustFingerprint() string {
	sum := sha256.Sum256(trustedKeysJSON)
	return hex.EncodeToString(sum[:])
}

func verifyManifest(raw, sig []byte, keys map[string]ed25519.PublicKey, channel, tag string) (Manifest, error) {
	var m Manifest
	if !utf8.Valid(raw) {
		return m, errors.New("update manifest is not UTF-8")
	}
	// Publisher signs the exact UTF-8 bytes: no envelope or domain prefix.
	value, err := base64.StdEncoding.Strict().DecodeString(strings.TrimSpace(string(sig)))
	trusted := false
	for _, key := range keys {
		if len(key) == ed25519.PublicKeySize && err == nil && ed25519.Verify(key, raw, value) {
			trusted = true
		}
	}
	if !trusted {
		return m, errors.New("update signature is not trusted")
	}
	if err := decodeStrict(raw, &m); err != nil {
		return m, errors.New("invalid update manifest")
	}
	if m.SchemaVersion != 1 || m.Channel != channel || !validReleaseTag(tag) ||
		!ValidSource(m.Version, m.SourceRef, m.SourceSHA, channel) ||
		(channel == "stable" && m.SourceRef != "refs/tags/"+tag) {
		return Manifest{}, errors.New("update manifest does not match release, channel, or source")
	}
	if len(m.Assets) != 7 {
		return Manifest{}, errors.New("invalid update artifact count")
	}
	seen := map[string]bool{}
	names := map[string]bool{}
	for _, a := range m.Assets {
		hash, err := hex.DecodeString(a.SHA256)
		suffix := ".tar.gz"
		if strings.HasPrefix(a.Platform, "windows-") {
			suffix = ".zip"
		}
		if err != nil || len(hash) != 32 || !validPlatform(a.Platform) ||
			!safeAsset.MatchString(a.Name) || !strings.HasSuffix(a.Name, suffix) ||
			seen[a.Platform] || names[a.Name] {
			return Manifest{}, errors.New("invalid update artifact")
		}
		seen[a.Platform], names[a.Name] = true, true
	}
	return m, nil
}

func decodeStrict(raw []byte, dest any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(dest); err != nil {
		return err
	}
	if !errors.Is(d.Decode(new(any)), io.EOF) {
		return errors.New("trailing JSON")
	}
	return nil
}

var safeTag = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._+/-]{0,199}$`)
var safeAsset = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._+-]{0,199}$`)
var sourceSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)
var branchRef = regexp.MustCompile(`^refs/heads/(?:prelease|prerelease)/([0-9]+\.[0-9]+\.[0-9]+)$`)

func validReleaseTag(tag string) bool {
	return safeTag.MatchString(tag) && !strings.Contains(tag, "..") && !strings.Contains(tag, "//") && !strings.HasSuffix(tag, "/")
}

func validPlatform(platform string) bool {
	switch platform {
	case "linux-amd64", "linux-arm64", "linux-armv7", "darwin-amd64", "darwin-arm64", "windows-amd64", "windows-arm64":
		return true
	default:
		return false
	}
}

// ValidSource binds a SemVer and full commit SHA to the allowed publisher stream.
// Prelease release tags are intentionally independent of their SemVer version.
func ValidSource(value, ref, sha, channel string) bool {
	v, ok := parseVersion(value)
	if !ok || !sourceSHA.MatchString(sha) {
		return false
	}
	if channel == "stable" {
		return v.pre == "" && ref == "refs/tags/v"+strings.TrimPrefix(value, "v")
	}
	if channel != "prelease" {
		return false
	}
	match := branchRef.FindStringSubmatch(ref)
	return match != nil && match[1] == fmt.Sprintf("%d.%d.%d", v.major, v.minor, v.patch)
}

// CompareVersions compares supported SemVer strings (with or without v).
func CompareVersions(a, b string) (int, error) {
	aa, okA := parseVersion(a)
	bb, okB := parseVersion(b)
	if !okA || !okB {
		return 0, errors.New("unrecognized update version")
	}
	return compareVersions(aa, bb), nil
}

func artifactName(version, os, arch, arm string) string {
	suffix := ".tar.gz"
	if os == "windows" {
		suffix = ".zip"
	}
	if arm != "" {
		arch += "v" + arm
	}
	return fmt.Sprintf("loom_%s_%s_%s%s", strings.TrimPrefix(version, "v"), os, arch, suffix)
}

type version struct {
	major, minor, patch uint64
	pre                 string
}

var versionPattern = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)

func parseVersion(s string) (version, bool) {
	m := versionPattern.FindStringSubmatch(s)
	if m == nil {
		return version{}, false
	}
	var v version
	for i, dst := range []*uint64{&v.major, &v.minor, &v.patch} {
		n, err := strconv.ParseUint(m[i+1], 10, 64)
		if err != nil {
			return version{}, false
		}
		*dst = n
	}
	v.pre = m[4]
	for _, id := range strings.Split(v.pre, ".") {
		if numeric(id) && len(id) > 1 && id[0] == '0' {
			return version{}, false
		}
	}
	return v, true
}

func numeric(s string) bool {
	return s != "" && strings.Trim(s, "0123456789") == ""
}

// compareVersions compares validated semvers, including numeric prerelease IDs
// without integer overflow. Build metadata and development builds are excluded.
func compareVersions(a, b version) int {
	for _, pair := range [][2]uint64{{a.major, b.major}, {a.minor, b.minor}, {a.patch, b.patch}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	if a.pre == b.pre {
		return 0
	}
	if a.pre == "" {
		return 1
	}
	if b.pre == "" {
		return -1
	}
	aa, bb := strings.Split(a.pre, "."), strings.Split(b.pre, ".")
	for i := 0; i < len(aa) && i < len(bb); i++ {
		x, y := aa[i], bb[i]
		if x == y {
			continue
		}
		xn, yn := numeric(x), numeric(y)
		if xn != yn {
			if xn {
				return -1
			}
			return 1
		}
		if xn && len(x) != len(y) {
			if len(x) < len(y) {
				return -1
			}
			return 1
		}
		return strings.Compare(x, y)
	}
	if len(aa) < len(bb) {
		return -1
	}
	return 1
}
