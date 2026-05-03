// Package tvdb provides a metadata provider for TheTVDB API v4.
//
// # Authentication
//
// TVDB v4 uses JWT (JSON Web Token) authentication, not static Bearer tokens.
// Tokens are obtained via the /login endpoint with an API key and optional PIN.
// Tokens are valid for 24 hours; the client automatically refreshes on 401 responses.
//
// Session-based JWT auth prevents abuse and allows per-user rate limits.
// The token is stored in-memory only; no persistent session state.
//
// # Configuration
//
// Config requires:
//   - APIKey: TheTVDB API key (https://www.thetvdb.com/api-information)
//   - PIN: Optional personal PIN for increased rate limits
//   - BaseURL: Defaults to "https://api4.thetvdb.com/v4"
//
// Example:
//
//	cfg := Config{
//	    APIKey: os.Getenv("LOOM_METADATA_TVDB_APIKEY"),
//	    PIN:    os.Getenv("LOOM_METADATA_TVDB_PIN"),
//	}
//	client := NewClient(cfg)
//
// # Rate Limiting
//
// TVDB enforces rate limits (HTTP 429). The client implements exponential
// backoff:
//
//   - Start: 1 second
//   - Max: 60 seconds
//   - Multiplier: 2x per retry
//
// The Retry-After header is respected if present. Rate limit errors include
// the recommended retry delay.
//
// # Concurrency
//
// The Client is safe for concurrent use. Token refresh is synchronized via
// sync.Mutex to prevent thundering herd on 401 responses.
//
// # Supported Endpoints
//
//   - GetSeries(ctx, tvdbID) — Get series metadata by TVDB ID
//   - SearchSeries(ctx, query, year) — Search for series by title
//   - FindSeries(ctx, title, externalIDs) — Implements MetadataProvider
//   - FindMovie(ctx, ...) — Not supported (returns error)
//   - FindEpisode(ctx, ...) — Requires episode TVDB ID (two-step lookup)
//
// # Partial Results
//
// Some TVDB responses may lack fields (e.g., IMDb ID, poster image).
// The client returns partial metadata; missing fields are left empty/nil.
// Callers should handle optional fields gracefully.
//
// # Error Handling
//
// The package defines typed errors:
//
//   - ErrCodeNotFound (404)
//   - ErrCodeUnauthorized (401)
//   - ErrCodeRateLimit (429)
//   - ErrCodeServerError (5xx)
//   - ErrCodeClientError (4xx)
//   - ErrCodeNetworkError (I/O)
//   - ErrCodeContextError (timeout/cancel)
//
// Use errors.As(err, &tvdb.ClientError{}) to inspect error details.
package tvdb
