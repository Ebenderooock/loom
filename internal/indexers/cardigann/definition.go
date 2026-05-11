package cardigann

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

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
	// ID is the Prowlarr-style identifier. Prowlarr uses `id:` where
	// Cardigann uses `site:`. When both are present, Site takes
	// precedence; when only ID is set it is copied to Site during
	// validation.
	ID string `yaml:"id,omitempty"`

	// Site is the short, kebab-case identifier (e.g. "exampletracker").
	// It is the document-level key used for cross-referencing inside
	// Loom (definition_id in the indexer config) and matches the
	// Cardigann convention.
	Site string `yaml:"site,omitempty"`

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

	// LegacyLinks are old URLs that no longer work. Prowlarr includes
	// these for migration purposes; Loom parses but ignores them.
	LegacyLinks []string `yaml:"legacylinks,omitempty"`

	// RequestDelay is a Prowlarr-specific rate-limit hint (ms between
	// requests). Enforced via throttle rate-limiting when > 0.
	RequestDelay int `yaml:"requestDelay,omitempty"`

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
	// category name → Newznab category name or numeric ID string.
	// Prowlarr uses string values (e.g. "Movies/HD"); legacy
	// Cardigann used numeric IDs. Both are accepted.
	Categories map[string]string `yaml:"categories,omitempty"`

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
	Name    string         `yaml:"name"`
	Type    string         `yaml:"type,omitempty"`
	Label   string         `yaml:"label,omitempty"`
	Default string         `yaml:"default,omitempty"`
	Options SettingOptions `yaml:"options,omitempty"`
}

// SettingOptions handles both Prowlarr's map format ({4: created, 7: seeders})
// and the legacy Cardigann slice format (["option1", "option2"]).
type SettingOptions struct {
	Map map[string]string
}

// UnmarshalYAML implements yaml.Unmarshaler so that options can be
// decoded from either a YAML mapping or a sequence.
func (o *SettingOptions) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.MappingNode:
		m := make(map[string]string)
		if err := value.Decode(&m); err != nil {
			return err
		}
		o.Map = m
	case yaml.SequenceNode:
		var s []string
		if err := value.Decode(&s); err != nil {
			return err
		}
		m := make(map[string]string, len(s))
		for _, v := range s {
			m[v] = v
		}
		o.Map = m
	case yaml.ScalarNode:
		if value.Tag == "!!null" || value.Value == "" {
			return nil
		}
		o.Map = map[string]string{value.Value: value.Value}
	default:
		return fmt.Errorf("unsupported YAML node kind %d for SettingOptions", value.Kind)
	}
	return nil
}

// Values returns the option keys in sorted order (useful for UI display).
func (o SettingOptions) Values() []string {
	if len(o.Map) == 0 {
		return nil
	}
	keys := make([]string, 0, len(o.Map))
	for k := range o.Map {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Login describes the form-based login handshake.
//
// Cardigann supports several login methods upstream (form, post,
// cookie, get). Loom currently implements `form` / `post` (alias) and
// `cookie`. Any other value triggers an unsupported-mode error at
// engine.Test() time so failures surface with a clear message rather
// than silently producing zero results.

// FlexStringSlice handles YAML values that can be either a scalar
// string or a sequence of strings (Prowlarr uses ["value"] for headers).
type FlexStringSlice []string

// UnmarshalYAML implements yaml.Unmarshaler for FlexStringSlice.
func (f *FlexStringSlice) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		*f = FlexStringSlice{value.Value}
	case yaml.SequenceNode:
		var s []string
		if err := value.Decode(&s); err != nil {
			return err
		}
		*f = FlexStringSlice(s)
	default:
		return fmt.Errorf("unsupported YAML node kind %d for FlexStringSlice", value.Kind)
	}
	return nil
}

// First returns the first value or empty string.
func (f FlexStringSlice) First() string {
	if len(f) > 0 {
		return f[0]
	}
	return ""
}

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

	// Message handles both Prowlarr's nested {selector: "..."} format
	// and the plain string used by legacy Cardigann definitions.
	Message FlexMessage `yaml:"message,omitempty"`
}

// FlexMessage handles Prowlarr's SelectorBlock format ({selector: "span.msg"})
// and the plain string format used by legacy Cardigann definitions.
type FlexMessage struct {
	Text     string // plain message text
	Selector string // CSS selector to extract message (Prowlarr format)
}

// UnmarshalYAML implements yaml.Unmarshaler for FlexMessage.
func (fm *FlexMessage) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		fm.Text = value.Value
	case yaml.MappingNode:
		var m struct {
			Selector string `yaml:"selector"`
			Text     string `yaml:"text"`
		}
		if err := value.Decode(&m); err != nil {
			return err
		}
		fm.Selector = m.Selector
		if m.Text != "" {
			fm.Text = m.Text
		}
	default:
		return fmt.Errorf("unsupported YAML node kind %d for FlexMessage", value.Kind)
	}
	return nil
}

// IsZero returns true if neither text nor selector is set.
func (fm FlexMessage) IsZero() bool {
	return fm.Text == "" && fm.Selector == ""
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
	// Paths lists candidate search endpoints.  The engine iterates
	// all paths, merging results and deduplicating by resolved URL.
	Paths []SearchPath `yaml:"paths,omitempty"`

	// Path is a Cardigann-compatible single-path shorthand.
	Path string `yaml:"path,omitempty"`

	// Method defaults to "get". "post" is supported.
	Method string `yaml:"method,omitempty"`

	// Inputs are the request parameters. Values are templated; see
	// engine.go for the available context.
	Inputs map[string]string `yaml:"inputs,omitempty"`

	// Headers are extra HTTP headers. Values can be a plain string or
	// a list (Prowlarr format); the FlexStringSlice type handles both.
	Headers map[string]FlexStringSlice `yaml:"headers,omitempty"`

	// KeywordsFilters transform the search term before it is
	// substituted into path templates and inputs (e.g. replacing
	// spaces with hyphens for URL-slug sites like EZTV).
	KeywordsFilters []Filter `yaml:"keywordsfilters,omitempty"`

	// Rows is the selector that yields one node per release.
	Rows RowsBlock `yaml:"rows"`

	// Fields maps a logical field name (title, download, size, …)
	// to a selector pipeline.
	Fields map[string]Field `yaml:"fields"`
}

// SearchPath is one candidate search endpoint.
type SearchPath struct {
	Path     string       `yaml:"path"`
	Method   string       `yaml:"method,omitempty"`
	Response FlexResponse `yaml:"response,omitempty"`
}

// FlexResponse handles Prowlarr's {type: json} map format and plain
// string format for the response field.
type FlexResponse struct {
	Type string
}

// UnmarshalYAML implements yaml.Unmarshaler for FlexResponse.
func (fr *FlexResponse) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		fr.Type = value.Value
	case yaml.MappingNode:
		var m struct {
			Type string `yaml:"type"`
		}
		if err := value.Decode(&m); err != nil {
			return err
		}
		fr.Type = m.Type
	default:
		return fmt.Errorf("unsupported YAML node kind %d for FlexResponse", value.Kind)
	}
	return nil
}

// RowsBlock is the selector that returns the per-release nodes.
type RowsBlock struct {
	// Selector is the CSS selector (HTML) or dot-path (JSON) that
	// locates the array of result items.
	Selector string `yaml:"selector"`

	// After is a Cardigann attribute that drops the first N rows of
	// each result page (used to skip a header row).
	After int `yaml:"after,omitempty"`

	// NoResultsMessage, when set, is checked against the response
	// body; if found, the engine returns 0 results instead of an
	// error. Prowlarr uses this for sites that show "no results"
	// in the HTML/JSON instead of an empty result set.
	NoResultsMessage string `yaml:"noResultsMessage,omitempty"`

	// Attribute names a nested array inside each row that should be
	// expanded into individual results (e.g. "torrents" in YTS where
	// each movie has multiple quality variants). JSON mode only.
	Attribute string `yaml:"attribute,omitempty"`

	// Multiple, when true combined with Attribute, produces one
	// result per attribute entry × parent row. JSON mode only.
	Multiple bool `yaml:"multiple,omitempty"`

	// MissingAttributeEqualsNoResults makes a missing Attribute
	// yield zero results instead of an error. JSON mode only.
	MissingAttributeEqualsNoResults bool `yaml:"missingAttributeEqualsNoResults,omitempty"`

	// Count optionally holds a selector whose value indicates the
	// total number of results. JSON mode only.
	Count *RowsCount `yaml:"count,omitempty"`

	// Filters apply to the node-set as a whole. Currently ignored.
	Filters []Filter `yaml:"filters,omitempty"`
}

// RowsCount locates a scalar count value in the response.
type RowsCount struct {
	Selector string `yaml:"selector"`
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
	Default   string            `yaml:"default,omitempty"`
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
