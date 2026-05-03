package throttle

// Config carries the three per-indexer dials operators can tune to be
// a polite client against any given tracker / Usenet provider. Zero
// values are treated as "use the package default" by Resolve, so
// callers can pass a partial Config and still get safe behaviour.
type Config struct {
	// PerMinute caps the token-bucket fill rate (requests per minute).
	// <= 0 means "use DefaultPerMinute".
	PerMinute int

	// Burst is the bucket capacity, i.e. how many requests can fire
	// back-to-back after a quiet period. <= 0 means "use DefaultBurst".
	Burst int

	// MaxRetries is the cap on retried attempts after the initial try
	// for retriable failures (429, 503, transient network errors).
	// 0 disables retries entirely; < 0 means "use DefaultMaxRetries".
	MaxRetries int
}

// Sensible defaults. Pegged conservatively low: most public trackers
// publish "be polite" guidance in the 30–120 req/min range, and the
// majority of Usenet providers will tolerate a small burst of five.
const (
	DefaultPerMinute  = 60
	DefaultBurst      = 5
	DefaultMaxRetries = 3
)

// Defaults returns the package-default Config. Useful for tests and
// for the rare caller that wants the value rather than re-deriving it.
func Defaults() Config {
	return Config{
		PerMinute:  DefaultPerMinute,
		Burst:      DefaultBurst,
		MaxRetries: DefaultMaxRetries,
	}
}

// Resolve returns a Config with every zero/negative field replaced by
// the package default. The MaxRetries field uses < 0 as the sentinel
// for "default" so that an explicit zero ("never retry") is honoured.
func Resolve(cfg Config) Config {
	out := cfg
	if out.PerMinute <= 0 {
		out.PerMinute = DefaultPerMinute
	}
	if out.Burst <= 0 {
		out.Burst = DefaultBurst
	}
	if out.MaxRetries < 0 {
		out.MaxRetries = DefaultMaxRetries
	}
	return out
}
