// Package musicbrainz provides a client for the MusicBrainz metadata API.
//
// MusicBrainz is an open music encyclopedia that collects music metadata
// (artists, albums, recordings, etc.). This package implements the MetadataProvider
// interface to integrate music metadata lookups into the Loom service.
//
// # Usage
//
//	client := musicbrainz.NewClient(musicbrainz.DefaultConfig())
//	
//	// Get artist metadata by MBID
//	artist, err := client.GetArtist(ctx, "12c6fc9b-c70d-45c0-8aab-75731bde6e56")
//	
//	// Search for artists
//	results, err := client.SearchArtist(ctx, "The Beatles", 0, 10)
//	
//	// Get album/release metadata
//	release, err := client.GetRelease(ctx, "9a2b12b4-1234-1234-1234-123456789abc")
//	
//	// Get track/recording metadata
//	recording, err := client.GetRecording(ctx, "abcd1234-abcd-1234-abcd-1234abcd1234")
//
// # Rate Limiting
//
// MusicBrainz has built-in rate limiting (1 request per second by default).
// This client enforces a 1-second throttle between consecutive requests.
// For 429 (Too Many Requests) responses, it implements exponential backoff
// (starting at 1s, doubling up to 60s, capped at 5 retries).
//
// # User-Agent Requirement
//
// MusicBrainz requires a descriptive User-Agent header to identify clients.
// The client automatically sets: "Loom/1.0 (metadata_service)"
//
// # Error Handling
//
// The client returns typed ClientError values with error codes:
//   - ErrCodeNotFound: 404 responses (entity not found)
//   - ErrCodeRateLimit: 429 responses (rate limited, includes Retry-After)
//   - ErrCodeClientError: 4xx responses (client errors)
//   - ErrCodeServerError: 5xx responses (server errors, retried)
//   - ErrCodeNetworkError: network I/O errors
//   - ErrCodeContextError: context cancellation/timeout
//
// # Concurrency
//
// The Client is safe for concurrent use. Rate limiting and request throttling
// are handled internally with sync.Mutex to ensure serialized request ordering.
//
// # API Endpoints
//
// Supported endpoints (see MusicBrainz API docs: https://musicbrainz.org/development/mmd):
//   - GET /artist/{mbid}: Fetch artist by MBID
//   - GET /release/{mbid}: Fetch release/album by MBID
//   - GET /recording/{mbid}: Fetch recording/track by MBID
//   - GET /artist?query=...: Search artists by query
//   - GET /release?query=...: Search releases/albums by query
//
// All requests accept JSON format via Accept: application/json header.
package musicbrainz
