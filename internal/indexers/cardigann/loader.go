package cardigann

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Loader reads Cardigann YAML definitions from a directory tree.
//
// One Loader per data dir is plenty: the loader caches by site/file
// id so repeated lookups are cheap. The cache is rebuilt on Reload;
// callers (typically the kind factory) call Reload at boot and
// whenever the operator drops a new file in.
type Loader struct {
	root string

	mu  sync.RWMutex
	all map[string]*Definition // keyed by definition id (basename without extension or `site`)
	src map[string]string      // id → absolute file path
}

// LoadError is returned when a single YAML file fails to parse or
// validate. The path/hint pair lets the caller surface a useful
// message; tests assert on Path so test cases stay readable.
type LoadError struct {
	Path string
	Hint string
	Err  error
}

// Error formats as a single-line message that includes file path and
// remediation hint. Cardigann definitions live one-per-file so the
// path is often enough to identify the offender.
func (e *LoadError) Error() string {
	if e.Hint == "" {
		return fmt.Sprintf("cardigann: %s: %v", e.Path, e.Err)
	}
	return fmt.Sprintf("cardigann: %s: %v (%s)", e.Path, e.Err, e.Hint)
}

// Unwrap returns the wrapped low-level error so errors.Is works.
func (e *LoadError) Unwrap() error { return e.Err }

// NewLoader creates a Loader rooted at dir. The directory is not
// touched until Reload is called.
func NewLoader(dir string) *Loader {
	return &Loader{
		root: dir,
		all:  map[string]*Definition{},
		src:  map[string]string{},
	}
}

// Root returns the directory the loader was constructed with.
func (l *Loader) Root() string { return l.root }

// Reload walks the loader's root recursively and parses every *.yml
// or *.yaml file. Returned slice contains every definition that
// loaded successfully; per-file failures appear in errs so the caller
// can log them without aborting the boot sequence.
//
// If the root does not exist, Reload returns (nil, nil) — operators
// who do not use Cardigann should not be punished with a startup
// warning.
func (l *Loader) Reload() (defs []*Definition, errs []error) {
	if l.root == "" {
		return nil, nil
	}
	info, err := os.Stat(l.root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, []error{fmt.Errorf("cardigann: stat %q: %w", l.root, err)}
	}
	if !info.IsDir() {
		return nil, []error{fmt.Errorf("cardigann: %q is not a directory", l.root)}
	}

	loaded := map[string]*Definition{}
	src := map[string]string{}

	walkErr := filepath.WalkDir(l.root, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			errs = append(errs, fmt.Errorf("cardigann: walk %q: %w", p, werr))
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !isYAMLFile(p) {
			return nil
		}
		def, lerr := LoadFile(p)
		if lerr != nil {
			errs = append(errs, lerr)
			return nil
		}
		id := definitionID(p, def)
		if existing, dup := loaded[id]; dup {
			errs = append(errs, &LoadError{
				Path: p,
				Hint: fmt.Sprintf("duplicate id %q (also in %s); rename one of the files", id, src[existing.Site]),
				Err:  errors.New("duplicate definition id"),
			})
			return nil
		}
		loaded[id] = def
		src[id] = p
		defs = append(defs, def)
		return nil
	})
	if walkErr != nil {
		errs = append(errs, fmt.Errorf("cardigann: walk: %w", walkErr))
	}

	l.mu.Lock()
	l.all = loaded
	l.src = src
	l.mu.Unlock()

	return defs, errs
}

// Get returns the definition stored under id, or false if absent.
// Callers should treat the returned pointer as read-only.
func (l *Loader) Get(id string) (*Definition, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	def, ok := l.all[id]
	return def, ok
}

// All returns a snapshot of every loaded definition keyed by id.
// The returned map is owned by the caller; mutating it does not
// affect the loader.
func (l *Loader) All() map[string]*Definition {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make(map[string]*Definition, len(l.all))
	for id, d := range l.all {
		out[id] = d
	}
	return out
}

// Path returns the file path the definition with id was loaded from.
// Empty when the id is unknown.
func (l *Loader) Path(id string) string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.src[id]
}

// LoadFile parses and validates a single YAML file.
//
// Validation errors are wrapped in *LoadError so the caller gets a
// path-tagged message; nil is returned only when the file parsed and
// passed every required-field check.
func LoadFile(path string) (*Definition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &LoadError{
			Path: path,
			Hint: "check file permissions and path spelling",
			Err:  err,
		}
	}
	def, err := ParseDefinition(data)
	if err != nil {
		return nil, &LoadError{
			Path: path,
			Hint: "see docs/indexers-cardigann.md for the YAML schema",
			Err:  err,
		}
	}
	if err := validate(def); err != nil {
		return nil, &LoadError{
			Path: path,
			Hint: "the file parsed but a required field was missing",
			Err:  err,
		}
	}
	return def, nil
}

// ParseDefinition decodes raw YAML bytes into a Definition without
// touching the filesystem. Useful for tests and for the eventual
// definition-repo sync (deferred — see ADR-0012).
func ParseDefinition(data []byte) (*Definition, error) {
	def := &Definition{}
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(false) // be lenient; upstream adds new fields constantly.
	if err := dec.Decode(def); err != nil {
		return nil, fmt.Errorf("decode yaml: %w", err)
	}
	if err := validate(def); err != nil {
		return nil, err
	}
	return def, nil
}

// validate checks the small set of fields the engine relies on. We
// keep the surface intentionally narrow: a definition that omits
// (say) `description` is fine, but one without `links` cannot work.
func validate(def *Definition) error {
	if def == nil {
		return errors.New("definition is nil")
	}
	// Prowlarr uses `id:` where Cardigann uses `site:`. Accept either.
	if strings.TrimSpace(def.Site) == "" && strings.TrimSpace(def.ID) != "" {
		def.Site = strings.TrimSpace(def.ID)
	}
	if strings.TrimSpace(def.Site) == "" {
		return errors.New("site (or id) is required")
	}
	if strings.TrimSpace(def.Name) == "" {
		return errors.New("name is required")
	}
	if len(def.Links) == 0 {
		return errors.New("links is required (at least one base URL)")
	}
	for i, link := range def.Links {
		if !strings.HasPrefix(link, "http://") && !strings.HasPrefix(link, "https://") {
			return fmt.Errorf("links[%d] %q must be an http(s) URL", i, link)
		}
	}
	if def.Search.Path == "" && len(def.Search.Paths) == 0 {
		return errors.New("search.path or search.paths is required")
	}
	if def.Search.Rows.Selector == "" {
		return errors.New("search.rows.selector is required")
	}
	if len(def.Search.Fields) == 0 {
		return errors.New("search.fields must declare at least one field")
	}
	return nil
}

// definitionID is the loader's canonical id for a definition. We
// prefer the file's basename (sans extension) so operators can
// override the upstream `site:` value by renaming the file — handy
// when running two flavours of the same tracker. Fall back to
// def.Site when no basename is available (in-memory tests).
func definitionID(path string, def *Definition) string {
	if path == "" {
		return strings.TrimSpace(def.Site)
	}
	base := filepath.Base(path)
	for _, ext := range []string{".yml", ".yaml"} {
		if strings.HasSuffix(base, ext) {
			base = strings.TrimSuffix(base, ext)
			break
		}
	}
	if base == "" {
		return strings.TrimSpace(def.Site)
	}
	return base
}

// LoadEmbedded merges definitions from an fs.FS (typically an embed.FS)
// into the loader. User-supplied definitions loaded from disk take
// precedence over embedded ones, so operators can override bundled
// definitions by placing a file with the same name in the definitions
// directory.
func (l *Loader) LoadEmbedded(fsys fs.FS) (defs []*Definition, errs []error) {
	walkErr := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			errs = append(errs, fmt.Errorf("cardigann: embedded walk %q: %w", p, werr))
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !isYAMLFile(p) {
			return nil
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			errs = append(errs, &LoadError{
				Path: "embedded:" + p,
				Hint: "unexpected read error on embedded file",
				Err:  err,
			})
			return nil
		}
		def, err := ParseDefinition(data)
		if err != nil {
			errs = append(errs, &LoadError{
				Path: "embedded:" + p,
				Hint: "bundled definition failed validation",
				Err:  err,
			})
			return nil
		}
		id := definitionID(p, def)

		l.mu.RLock()
		_, exists := l.all[id]
		l.mu.RUnlock()

		if exists {
			// Disk definition takes precedence; skip the embedded one.
			return nil
		}

		l.mu.Lock()
		// Double-check after acquiring write lock.
		if _, exists := l.all[id]; !exists {
			l.all[id] = def
			l.src[id] = "embedded:" + p
			defs = append(defs, def)
		}
		l.mu.Unlock()
		return nil
	})
	if walkErr != nil {
		errs = append(errs, fmt.Errorf("cardigann: embedded walk: %w", walkErr))
	}
	return defs, errs
}

// isYAMLFile is true for paths whose lowercase extension is .yml or
// .yaml. We compare lowercase to be friendly to case-insensitive
// filesystems (macOS, Windows).
func isYAMLFile(p string) bool {
	ext := strings.ToLower(filepath.Ext(p))
	return ext == ".yml" || ext == ".yaml"
}
