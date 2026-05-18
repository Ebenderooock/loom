package proxies

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Service wraps a Repository with config validation, transport
// caching, and a small TestProxy probe. It implements the
// indexers.TransportProvider interface (TransportFor) so cmd/loom can
// register it via indexers.SetTransportProvider.
type Service struct {
	repo     Repository
	provider *Provider
	fs       *FlareSolverrClient
	logger   *slog.Logger

	probeURL  string
	probeHTTP *http.Client
}

// ServiceOptions configures the Service.
type ServiceOptions struct {
	Repository            Repository
	Logger                *slog.Logger
	FlareSolverrTimeout   time.Duration
	FlareSolverrHTTPClient *http.Client
	TestProbeURL          string
}

// NewService builds a Service. Repository is required.
func NewService(opts ServiceOptions) (*Service, error) {
	if opts.Repository == nil {
		return nil, errors.New("proxies.NewService: Repository is required")
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	probe := opts.TestProbeURL
	if strings.TrimSpace(probe) == "" {
		probe = "https://www.google.com/generate_204"
	}
	fs := NewFlareSolverrClient(opts.FlareSolverrHTTPClient, opts.FlareSolverrTimeout)
	prov := NewProvider(opts.Repository, fs)
	return &Service{
		repo:      opts.Repository,
		provider:  prov,
		fs:        fs,
		logger:    logger,
		probeURL:  probe,
		probeHTTP: &http.Client{Timeout: 15 * time.Second},
	}, nil
}

// TransportFor implements indexers.TransportProvider.
func (s *Service) TransportFor(proxyID string) (http.RoundTripper, error) {
	return s.provider.TransportFor(proxyID)
}

// List returns all rows.
func (s *Service) List(ctx context.Context) ([]Proxy, error) {
	return s.repo.List(ctx)
}

// Get returns one row.
func (s *Service) Get(ctx context.Context, id string) (Proxy, error) {
	return s.repo.Get(ctx, id)
}

// Create validates and persists a new proxy. ID is generated when
// empty.
func (s *Service) Create(ctx context.Context, p Proxy) (Proxy, error) {
	if strings.TrimSpace(string(p.Kind)) == "" {
		return Proxy{}, &ErrValidation{Msg: "kind is required"}
	}
	if strings.TrimSpace(p.Name) == "" {
		return Proxy{}, &ErrValidation{Msg: "name is required"}
	}
	canonical, err := ValidateConfig(p.Kind, p.Config)
	if err != nil {
		return Proxy{}, &ErrValidation{Msg: err.Error()}
	}
	p.Config = canonical
	if strings.TrimSpace(p.ID) == "" {
		p.ID = generateID(p.Kind, p.Name)
	}
	out, err := s.repo.Create(ctx, p)
	if err != nil {
		return Proxy{}, err
	}
	s.provider.Invalidate(out.ID)
	return out, nil
}

// Replace overwrites a proxy by ID.
func (s *Service) Replace(ctx context.Context, p Proxy) (Proxy, error) {
	if strings.TrimSpace(p.ID) == "" {
		return Proxy{}, &ErrValidation{Msg: "id is required"}
	}
	if strings.TrimSpace(string(p.Kind)) == "" {
		return Proxy{}, &ErrValidation{Msg: "kind is required"}
	}
	if strings.TrimSpace(p.Name) == "" {
		return Proxy{}, &ErrValidation{Msg: "name is required"}
	}
	canonical, err := ValidateConfig(p.Kind, p.Config)
	if err != nil {
		return Proxy{}, &ErrValidation{Msg: err.Error()}
	}
	p.Config = canonical
	out, err := s.repo.Replace(ctx, p)
	if err != nil {
		return Proxy{}, err
	}
	s.provider.Invalidate(out.ID)
	return out, nil
}

// Patch applies a partial update.
func (s *Service) Patch(ctx context.Context, patch Patch) (Proxy, error) {
	if strings.TrimSpace(patch.ID) == "" {
		return Proxy{}, &ErrValidation{Msg: "id is required"}
	}
	if patch.Config != nil {
		// Need the kind to validate. Use the patch's kind when set,
		// else load from the existing row.
		kind := Kind("")
		if patch.Kind != nil {
			kind = *patch.Kind
		} else {
			existing, err := s.repo.Get(ctx, patch.ID)
			if err != nil {
				return Proxy{}, err
			}
			kind = existing.Kind
		}
		canonical, err := ValidateConfig(kind, *patch.Config)
		if err != nil {
			return Proxy{}, &ErrValidation{Msg: err.Error()}
		}
		patch.Config = &canonical
	}
	out, err := s.repo.Patch(ctx, patch)
	if err != nil {
		return Proxy{}, err
	}
	s.provider.Invalidate(out.ID)
	return out, nil
}

// Delete refuses with ErrInUse when any indexer still references id.
func (s *Service) Delete(ctx context.Context, id string) error {
	users, err := s.repo.IndexerIDsUsing(ctx, id)
	if err != nil {
		return err
	}
	if len(users) > 0 {
		return &ErrInUse{IndexerIDs: users}
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.provider.Invalidate(id)
	return nil
}

// TestProxy issues a HEAD/GET against the configured probe URL using
// the proxy's transport. For FlareSolverr proxies it issues a
// `sessions.list` ping instead, since the probe URL would have to be
// fetched through the protected upstream which may not exist.
func (s *Service) TestProxy(ctx context.Context, id string) (TestResult, error) {
	row, err := s.repo.Get(ctx, id)
	if err != nil {
		return TestResult{}, err
	}
	if !row.Enabled {
		return TestResult{}, errors.New("proxy is disabled")
	}
	start := time.Now()

	if row.Kind == KindFlareSolverr {
		cfg, perr := ParseFlareSolverrConfig(row.Config)
		if perr != nil {
			return TestResult{}, perr
		}
		if err := s.fs.Ping(ctx, cfg); err != nil {
			return TestResult{OK: false, Error: err.Error(), DurationMS: time.Since(start).Milliseconds()}, nil
		}
		return TestResult{OK: true, DurationMS: time.Since(start).Milliseconds(), Detail: "flaresolverr sessions.list"}, nil
	}

	rt, err := BuildTransport(row, s.fs)
	if err != nil {
		return TestResult{}, err
	}
	cli := &http.Client{Transport: rt, Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.probeURL, nil)
	if err != nil {
		return TestResult{}, err
	}
	resp, err := cli.Do(req)
	if err != nil {
		return TestResult{OK: false, Error: err.Error(), DurationMS: time.Since(start).Milliseconds()}, nil
	}
	defer resp.Body.Close()
	dur := time.Since(start).Milliseconds()
	return TestResult{
		OK:         resp.StatusCode >= 200 && resp.StatusCode < 400,
		StatusCode: resp.StatusCode,
		DurationMS: dur,
		Detail:     fmt.Sprintf("GET %s", s.probeURL),
	}, nil
}

// TestResult is returned by POST /api/v1/proxies/{id}/test.
type TestResult struct {
	OK         bool   `json:"ok"`
	StatusCode int    `json:"status_code,omitempty"`
	DurationMS int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

// rawConfig is a tiny adapter so handlers can hand a struct to the
// service without re-marshalling.
type rawConfig = json.RawMessage

// generateID derives a stable, URL-safe slug from kind + name when
// the caller doesn't supply one. Mirrors indexers.generateID.
func generateID(kind Kind, name string) string {
	prefix := strings.TrimSpace(string(kind))
	if prefix == "" {
		prefix = "proxy"
	}
	slug := strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	prevDash := false
	for _, r := range slug {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.TrimRight(b.String(), "-")
	if out == "" {
		out = fmt.Sprintf("x%d", time.Now().UnixNano())
	}
	return prefix + "-" + out
}
