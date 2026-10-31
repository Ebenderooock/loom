package music

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
)

// Metadata source modes.
const (
	MetadataSourceMusicBrainz = "musicbrainz"
	MetadataSourceCustom      = "custom"
)

// MetadataSource selects the MusicBrainz-compatible endpoint used for music
// metadata. AuthHeaderValue is write-only and never returned by the API.
type MetadataSource struct {
	Mode            string `json:"mode"`
	BaseURL         string `json:"base_url"`
	RateLimitMS     int    `json:"rate_limit_ms"`
	AuthHeaderName  string `json:"auth_header_name"`
	AuthHeaderValue string `json:"-"`
	AuthHeaderSet   bool   `json:"auth_header_set"`
	SearchOnPublic  bool   `json:"search_on_public"`
}

// DefaultMetadataSource is the public MusicBrainz API.
func DefaultMetadataSource() MetadataSource {
	return MetadataSource{Mode: MetadataSourceMusicBrainz, SearchOnPublic: true}
}

// MetadataSourceUpdate is the request body for saving the metadata source.
// An empty AuthHeaderValue keeps the stored value unless ClearAuthHeader is set.
type MetadataSourceUpdate struct {
	Mode            string `json:"mode"`
	BaseURL         string `json:"base_url"`
	RateLimitMS     int    `json:"rate_limit_ms"`
	AuthHeaderName  string `json:"auth_header_name"`
	AuthHeaderValue string `json:"auth_header_value"`
	ClearAuthHeader bool   `json:"clear_auth_header"`
	SearchOnPublic  bool   `json:"search_on_public"`
}

// MetadataSourceTestResult reports whether an endpoint answers lookups and,
// when it serves search, artist search.
type MetadataSourceTestResult struct {
	OK           bool   `json:"ok"`
	LookupOK     bool   `json:"lookup_ok"`
	LookupError  string `json:"lookup_error,omitempty"`
	SearchTested bool   `json:"search_tested"`
	SearchOK     bool   `json:"search_ok"`
	SearchError  string `json:"search_error,omitempty"`
}

// MetadataSourceApplier applies a source to the live metadata provider.
type MetadataSourceApplier interface {
	Apply(src MetadataSource)
	Test(ctx context.Context, src MetadataSource) MetadataSourceTestResult
}

// MetadataSourceStore persists the single metadata source row.
type MetadataSourceStore interface {
	Get(ctx context.Context) (MetadataSource, error)
	Save(ctx context.Context, src MetadataSource) error
}

// NewMetadataSourceStore returns a SQL-backed store.
func NewMetadataSourceStore(db *sql.DB, engine ...string) MetadataSourceStore {
	postgres := len(engine) > 0 && engine[0] == "postgres"
	return &sqlMetadataSourceStore{db: &musicDB{raw: db, postgres: postgres}}
}

type sqlMetadataSourceStore struct {
	db *musicDB
}

func (s *sqlMetadataSourceStore) Get(ctx context.Context) (MetadataSource, error) {
	src := DefaultMetadataSource()
	err := s.db.QueryRowContext(ctx, `SELECT mode, base_url, rate_limit_ms, auth_header_name, auth_header_value, search_on_public
		FROM music_metadata_source WHERE id = 1`).Scan(
		&src.Mode, &src.BaseURL, &src.RateLimitMS, &src.AuthHeaderName, &src.AuthHeaderValue, &src.SearchOnPublic)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultMetadataSource(), nil
	}
	if err != nil {
		return MetadataSource{}, err
	}
	src.AuthHeaderSet = src.AuthHeaderValue != ""
	return src, nil
}

func (s *sqlMetadataSourceStore) Save(ctx context.Context, src MetadataSource) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO music_metadata_source
		(id, mode, base_url, rate_limit_ms, auth_header_name, auth_header_value, search_on_public)
		VALUES (1, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET mode = excluded.mode, base_url = excluded.base_url,
			rate_limit_ms = excluded.rate_limit_ms, auth_header_name = excluded.auth_header_name,
			auth_header_value = excluded.auth_header_value, search_on_public = excluded.search_on_public`,
		src.Mode, src.BaseURL, src.RateLimitMS, src.AuthHeaderName, src.AuthHeaderValue, src.SearchOnPublic)
	return err
}

// MetadataSourceService validates, stores and applies the metadata source.
type MetadataSourceService struct {
	mu      sync.Mutex
	store   MetadataSourceStore
	applier MetadataSourceApplier
}

// NewMetadataSourceService builds the service.
func NewMetadataSourceService(store MetadataSourceStore, applier MetadataSourceApplier) *MetadataSourceService {
	return &MetadataSourceService{store: store, applier: applier}
}

// Get returns the stored source.
func (s *MetadataSourceService) Get(ctx context.Context) (MetadataSource, error) {
	return s.store.Get(ctx)
}

// ApplyStored loads the stored source and applies it to the provider.
func (s *MetadataSourceService) ApplyStored(ctx context.Context) (MetadataSource, error) {
	src, err := s.store.Get(ctx)
	if err != nil {
		return MetadataSource{}, err
	}
	s.applier.Apply(src)
	return src, nil
}

// Update validates and saves the source, then applies it.
func (s *MetadataSourceService) Update(ctx context.Context, upd MetadataSourceUpdate) (MetadataSource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.store.Get(ctx)
	if err != nil {
		return MetadataSource{}, err
	}
	src, err := s.resolve(current, upd)
	if err != nil {
		return MetadataSource{}, err
	}
	if err := s.store.Save(ctx, src); err != nil {
		return MetadataSource{}, err
	}
	s.applier.Apply(src)
	return src, nil
}

// Test checks an unsaved source. An empty auth header value reuses the
// stored one so admins can test without re-entering the secret.
func (s *MetadataSourceService) Test(ctx context.Context, upd MetadataSourceUpdate) (MetadataSourceTestResult, error) {
	current, err := s.store.Get(ctx)
	if err != nil {
		return MetadataSourceTestResult{}, err
	}
	src, err := s.resolve(current, upd)
	if err != nil {
		return MetadataSourceTestResult{}, err
	}
	return s.applier.Test(ctx, src), nil
}

func (s *MetadataSourceService) resolve(current MetadataSource, upd MetadataSourceUpdate) (MetadataSource, error) {
	mode := strings.TrimSpace(upd.Mode)
	if mode == "" {
		mode = MetadataSourceMusicBrainz
	}
	switch mode {
	case MetadataSourceMusicBrainz:
		// Keep custom fields so switching back restores them.
		src := current
		src.Mode = MetadataSourceMusicBrainz
		return src, nil
	case MetadataSourceCustom:
	default:
		return MetadataSource{}, fmt.Errorf("%w: mode must be %q or %q", ErrInvalid, MetadataSourceMusicBrainz, MetadataSourceCustom)
	}

	baseURL := strings.TrimRight(strings.TrimSpace(upd.BaseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return MetadataSource{}, fmt.Errorf("%w: base_url must be an http or https URL", ErrInvalid)
	}
	if upd.RateLimitMS < 0 || upd.RateLimitMS > 60_000 {
		return MetadataSource{}, fmt.Errorf("%w: rate_limit_ms must be between 0 and 60000", ErrInvalid)
	}
	headerName := strings.TrimSpace(upd.AuthHeaderName)
	if headerName != "" && !validHeaderName(headerName) {
		return MetadataSource{}, fmt.Errorf("%w: auth_header_name is not a valid HTTP header name", ErrInvalid)
	}
	headerValue := current.AuthHeaderValue
	switch {
	case upd.ClearAuthHeader:
		headerValue = ""
	case upd.AuthHeaderValue != "":
		headerValue = upd.AuthHeaderValue
	}
	if strings.ContainsAny(headerValue, "\r\n") {
		return MetadataSource{}, fmt.Errorf("%w: auth_header_value must not contain line breaks", ErrInvalid)
	}
	if headerName == "" {
		headerValue = ""
	}
	return MetadataSource{
		Mode:            MetadataSourceCustom,
		BaseURL:         baseURL,
		RateLimitMS:     upd.RateLimitMS,
		AuthHeaderName:  http.CanonicalHeaderKey(headerName),
		AuthHeaderValue: headerValue,
		AuthHeaderSet:   headerValue != "",
		SearchOnPublic:  upd.SearchOnPublic,
	}, nil
}

// validHeaderName reports whether name is an RFC 9110 token.
func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		isAlnum := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		if !isAlnum && !strings.ContainsRune("!#$%&'*+-.^_`|~", r) {
			return false
		}
	}
	return true
}

// MetadataSourceRouter serves GET /, PUT / and POST /test. Mount it behind
// admin authorization: it controls where Loom sends metadata requests.
func MetadataSourceRouter(svc *MetadataSourceService) chi.Router {
	r := chi.NewRouter()
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		src, err := svc.Get(r.Context())
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, src)
	})
	r.Put("/", func(w http.ResponseWriter, r *http.Request) {
		var upd MetadataSourceUpdate
		if err := decodeMetadataSource(r, &upd); err != nil {
			writeErr(w, err)
			return
		}
		src, err := svc.Update(r.Context(), upd)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, src)
	})
	r.Post("/test", func(w http.ResponseWriter, r *http.Request) {
		var upd MetadataSourceUpdate
		if err := decodeMetadataSource(r, &upd); err != nil {
			writeErr(w, err)
			return
		}
		res, err := svc.Test(r.Context(), upd)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	return r
}

func decodeMetadataSource(r *http.Request, upd *MetadataSourceUpdate) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(upd); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return nil
}
