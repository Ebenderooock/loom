// Package tmdb provides a HTTP client for The Movie Database (TMDb) API.
//
// The Client fetches movie, TV series, and episode metadata from TMDb with
// built-in exponential backoff for rate limiting (429 responses).
//
// # Configuration
//
// Clients require a TMDb API key, available for free at https://www.themoviedb.org/settings/api.
//
// # Basic Usage
//
//	config := tmdb.Config{
//		APIKey:  os.Getenv("TMDB_API_KEY"),
//		BaseURL: "https://api.themoviedb.org/3",
//		Timeout: 10 * time.Second,
//	}
//	client := tmdb.NewClient(config)
//
//	// Get a movie by TMDb ID
//	movie, err := client.GetMovie(ctx, 550)  // Fight Club
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Printf("Title: %s, Year: %d\n", movie.Title, movie.Year)
//
//	// Search for movies
//	results, err := client.SearchMovie(ctx, "The Matrix", 1999)
//	if err != nil {
//		log.Fatal(err)
//	}
//	for _, m := range results {
//		fmt.Printf("- %s (%d)\n", m.Title, m.Year)
//	}
//
// # Provider Integration
//
// The Provider type implements the metadata.MetadataProvider interface,
// allowing it to be used with the metadata Service for cascading lookups.
//
//	provider := tmdb.NewProvider(client)
//	// Use provider with metadata.Service for integrated caching and fallback
//
// # Rate Limiting
//
// TMDb enforces a rate limit of 40 requests per 10 seconds. When the API
// returns a 429 (Too Many Requests) response, the client will:
//
// 1. Respect the Retry-After header if present
// 2. Otherwise, apply exponential backoff (1s → 2s → 4s → ... → 60s max)
// 3. Retry the request automatically within the request context
//
// If the context deadline is exceeded during backoff, the request fails
// immediately with a context error.
//
// # Error Handling
//
// The client returns typed errors (ClientError) for distinguishing between
// different failure modes:
//
//   - ErrCodeNotFound (404): Resource does not exist on TMDb
//   - ErrCodeUnauthorized (401): Invalid or missing API key
//   - ErrCodeRateLimit (429): Rate limit exceeded; includes Retry-After
//   - ErrCodeServerError (5xx): TMDb server error; automatic retry with backoff
//   - ErrCodeClientError (4xx other): Request error (bad parameters, etc.)
//   - ErrCodeNetworkError: Network-level error (connection refused, DNS, etc.)
//   - ErrCodeContextError: Context cancelled or deadline exceeded
//
// Example:
//
//	movie, err := client.GetMovie(ctx, 123)
//	if err != nil {
//		if ce, ok := err.(*ClientError); ok {
//			switch ce.Code {
//			case ErrCodeNotFound:
//				fmt.Println("Movie not found on TMDb")
//			case ErrCodeRateLimit:
//				fmt.Printf("Rate limited; retry after %d seconds\n", ce.RetryAfter)
//			case ErrCodeUnauthorized:
//				fmt.Println("Invalid API key")
//			default:
//				fmt.Printf("Error: %v\n", err)
//			}
//		}
//	}
//
// # Concurrent Safety
//
// The Client is safe for concurrent use by multiple goroutines.
// The underlying http.Client shares a connection pool, and all request
// handling is stateless (except for per-request backoff state).
package tmdb
