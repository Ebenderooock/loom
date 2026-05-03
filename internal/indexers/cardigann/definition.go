package cardigann

// Definition mirrors a single Cardigann YAML file. Field names and
// nesting follow the upstream schema documented at
// https://github.com/Cardigann/cardigann/blob/master/docs/definitions.md
// so that existing community definitions parse without translation.
//
// Loom does NOT implement every upstream feature; unsupported sections
// are still parsed (so Loom can warn rather than reject), but they do
// not influence runtime behaviour. See docs/indexers-cardigann.md for
// the supported / deferred matrix and the engine.go implementation.
type Definition struct {
	// Site is the short, kebab-case identifier (e.g. "exampletracker").
	// It is the document-level key used for cross-referencing inside
	// Loom (definition_id in the indexer config) and matches the
	// Cardigann convention.
	Site string `yaml:"site"`

	// Name is the human-readable tracker name shown in the UI.
	Name string `yaml:"name"`

	// Description is an optional one-liner about the tracker.
	Description string `yaml:"description,omitempty"`

	// Language follows BCP-47 ("en-us"). Informational only.
	Language string `yaml:"language,omitempty"`

	// Encoding is the source HTML encoding (e.g. "UTF-8"). The engine
	// always decodes via Go's net/http defaults today; this field is
	// captured for completeness but ignored.
	Encoding string `yaml:"encoding,omitempty"`

	// Type categorises the tracker as "public", "private", or
	// "semi-private". Informational only.
	Type string `yaml:"type,omitempty"`

	// Links are the candidate base URLs. The engine uses Links[0] as
	// the working base URL; failover across links is deferred.
	Links []string `yaml:"links"`

	// Caps describes search modes and the per-tracker → Newznab
	// category mapping. Surfaced via Indexer.Caps().
	Caps Caps `yaml:"caps"`

	// Settings declares the per-tracker credential fields the operator
	// must populate (username, password, passkey, cookie). The engine
	// reads the operator-supplied values from indexer config and
	// substitutes them into login/search templates.
	Settings []Setting `yaml:"settings,omitempty"`

	// Login describes the form-login flow. Optional: public trackers
	// omit this entirely.
	Login *Login `yaml:"login,omitempty"`

	// Search defines the request shape and the row/field selectors.
	Search Search `yaml:"search"`

	// Download is an optional override for how the .torrent / .nzb
	// link is resolved. When omitted the engine treats the `download`
	// field on each row as the final URL.
	Download *Download `yaml:"download,omitempty"`

	// Ratio is an upstream ratio-policy block. Loom does not enforce
	// ratio today; the field is parsed and ignored.
	Ratio *Ratio `yaml:"ratio,omitempty"`
}

// Caps mirrors Cardigann's `caps:` block.
type Caps struct {
	// Categories is the legacy form: a flat map of tracker-side
	// category name → Newznab numeric ID.
	Categories map[string]int `yaml:"categories,omitempty"`

	// CategoryMappings is the modern form: a list of {id, cat}
	// entries linking a tracker-specific category id to a
	// Newznab-style category name (e.g. "Movies/HD"). Loom resolves
	// the name to a numeric Newznab ID via knownCategoryNames.
	CategoryMappings []CategoryMapping `yaml:"categorymappings,omitempty"`

	// Modes lists the supported search modes ("search", "tv-search",
	// "movie-search") and the parameter keys the tracker accepts.
	Modes map[string][]string `yaml:"modes,omitempty"`
}

// CategoryMapping is one row of the categorymappings array.
type CategoryMapping struct {
	// ID is the tracker's own category id (e.g. "12"). Strings, not
	// ints, because some trackers use non-numeric tags.
	ID string `yaml:"id"`

	// Cat is the Newznab category name (e.g. "Movies/HD"). The engine
	// maps it to the Newznab numeric ID via knownCategoryNames.
	Cat string `yaml:"cat"`

	// Desc is a free-text description shown in operator UIs.
	Desc string `yaml:"desc,omitempty"`
}

// Setting describes one operator-supplied credential or option.
type Setting struct {
	Name    string   `yaml:"name"`
	Type    string   `yaml:"type,omitempty"`
	Label   string   `yaml:"label,omitempty"`
	Default string   `yaml:"default,omitempty"`
	Options []string `yaml:"options,omitempty"`
}

// Login describes the form-based login handshake.
//
// Cardigann supports several login methods upstream (form, post,
// cookie, get). Loom currently implements `form` / `post` (alias) and
// `cookie`. Any other value triggers an unsupported-mode error at
// engine.Test() time so failures surface with a clear message rather
// than silently producing zero results.
type Login struct {
	// Path is the login endpoint relative to the site base URL. It
	// MUST be set for form/post logins.
	Path string `yaml:"path,omitempty"`

	// Method is "form", "post", "cookie", or "get". When empty the
	// engine assumes "form".
	Method string `yaml:"method,omitempty"`

	// Inputs map form field names to template strings. Templates may
	// reference {{ .Config.username }} and so on (see engine.go).
	Inputs map[string]string `yaml:"inputs,omitempty"`

	// Error is a list of selectors that, if matched in the response,
	// indicate a failed login. The selector's text becomes the
	// returned error message.
	Error []ErrorBlock `yaml:"error,omitempty"`

	// Test describes the post-login probe Cardigann uses to verify
	// the session cookie was accepted.
	Test LoginTest `yaml:"test,omitempty"`
}

// ErrorBlock describes one error selector pattern.
type ErrorBlock struct {
	// Selector is a CSS selector applied to the response document.
	// When it matches, the trimmed text content becomes the login
	// error message.
	Selector string `yaml:"selector"`

	// Message is an optional override surfaced verbatim instead of
	// the selector's text content.
	Message string `yaml:"message,omitempty"`
}

// LoginTest describes the post-login verification probe.
type LoginTest struct {
	// Path is fetched after the login POST; success means the
	// selector below matches.
	Path string `yaml:"path,omitempty"`

	// Selector must match for the login to be considered successful
	// (e.g. an "a[href*=logout]" link).
	Selector string `yaml:"selector,omitempty"`
}

// Search describes the search request and row extraction.
type Search struct {
	// Paths lists candidate search endpoints. The engine uses
	// Paths[0] today; failover is deferred.
	Paths []SearchPath `yaml:"paths,omitempty"`

	// Path is a Cardigann-compatible single-path shorthand.
	Path string `yaml:"path,omitempty"`

	// Method defaults to "get". "post" is supported.
	Method string `yaml:"method,omitempty"`

	// Inputs are the request parameters. Values are templated; see
	// engine.go for the available context.
	Inputs map[string]string `yaml:"inputs,omitempty"`

	// Headers are extra HTTP headers (templated values).
	Headers map[string]string `yaml:"headers,omitempty"`

	// Rows is the selector that yields one node per release.
	Rows RowsBlock `yaml:"rows"`

	// Fields maps a logical field name (title, download, size, …)
	// to a selector pipeline.
	Fields map[string]Field `yaml:"fields"`
}

// SearchPath is one candidate search endpoint.
type SearchPath struct {
	Path     string `yaml:"path"`
	Method   string `yaml:"method,omitempty"`
	Response string `yaml:"response,omitempty"`
}

// RowsBlock is the selector that returns the per-release nodes.
type RowsBlock struct {
	// Selector is the CSS selector or XPath expression. XPath is
	// detected by a leading "/" or "(" character.
	Selector string `yaml:"selector"`

	// After is a Cardigann attribute that drops the first N rows of
	// each result page (used to skip a header row).
	After int `yaml:"after,omitempty"`

	// Filters apply to the node-set as a whole. Currently ignored.
	Filters []Filter `yaml:"filters,omitempty"`
}

// Field is one extraction recipe inside `fields:`.
//
// A field has a Selector (CSS or XPath), an optional Attribute (when
// set, the attribute value is taken instead of the inner text), and a
// Filters chain that post-processes the extracted string. Cardigann
// also supports `text:` (a literal value) and `case:` (conditional
// extraction); the case shape is parsed but only the simple
// selector/attribute path is implemented today.
type Field struct {
	Selector  string            `yaml:"selector,omitempty"`
	Attribute string            `yaml:"attribute,omitempty"`
	Text      string            `yaml:"text,omitempty"`
	Remove    string            `yaml:"remove,omitempty"`
	Filters   []Filter          `yaml:"filters,omitempty"`
	Case      map[string]string `yaml:"case,omitempty"`
	Optional  bool              `yaml:"optional,omitempty"`
}

// Filter is one step in a Cardigann field's filter chain.
//
// The Args field is intentionally `[]any` because upstream allows
// either positional ([" ", ""]) or named (key: value) arguments
// depending on the filter. The engine's applyFilter function does the
// per-name interpretation.
type Filter struct {
	Name string `yaml:"name"`
	Args any    `yaml:"args,omitempty"`
}

// Download is the optional `download:` block.
type Download struct {
	// Selector lets the definition point at a download anchor inside
	// the *details* page when the row-level field is not enough.
	// Loom does not yet fetch the details page; this is reserved.
	Selector string `yaml:"selector,omitempty"`

	// Method is "get" (default) or "post".
	Method string `yaml:"method,omitempty"`

	// Infohash points at an `<input>` carrying the BitTorrent
	// infohash. Reserved for future use.
	Infohash *Field `yaml:"infohash,omitempty"`
}

// Ratio is the optional ratio-policy block. Loom never enforces it.
type Ratio struct {
	Path     string `yaml:"path,omitempty"`
	Selector string `yaml:"selector,omitempty"`
	Filters  []Filter `yaml:"filters,omitempty"`
}
