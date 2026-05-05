package cardigann

import (
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// applyFilters runs the upstream filter chain over a value. Cardigann
// ships dozens of filters; we implement the ones our reference test
// definitions and the public tracker corpus we sampled actually use:
//
//	trim, lowercase, uppercase, replace, regexp, querystring, prepend,
//	append, split, join.
//
// Unknown filter names are passed through unchanged so a definition
// with an unsupported step still yields a usable value rather than an
// empty string. Operators see warnings about that in the docs page.
func applyFilters(in string, filters []Filter) string {
	v := in
	for _, f := range filters {
		v = applyFilter(v, f)
	}
	return strings.TrimSpace(v)
}

// applyFilter dispatches one filter step. Argument shapes vary across
// filters so we type-switch on the parsed YAML value.
func applyFilter(v string, f Filter) string {
	switch strings.ToLower(strings.TrimSpace(f.Name)) {
	case "trim":
		s := stringArg(f.Args)
		if s != "" {
			return strings.Trim(v, s)
		}
		return strings.TrimSpace(v)
	case "lowercase", "tolower":
		return strings.ToLower(v)
	case "uppercase", "toupper":
		return strings.ToUpper(v)
	case "replace":
		args := stringSliceArg(f.Args)
		if len(args) >= 2 {
			return strings.ReplaceAll(v, args[0], args[1])
		}
		return v
	case "regexp", "re_replace":
		return applyRegexp(v, f.Args)
	case "querystring":
		return queryParam(v, stringArg(f.Args))
	case "prepend":
		return stringArg(f.Args) + v
	case "append":
		return v + stringArg(f.Args)
	case "split":
		return applySplit(v, f.Args)
	case "join":
		args := stringSliceArg(f.Args)
		if len(args) >= 1 {
			return strings.Join(strings.Fields(v), args[0])
		}
		return v
	case "urldecode":
		decoded, err := url.QueryUnescape(v)
		if err != nil {
			return v
		}
		return decoded
	case "urlencode":
		return url.QueryEscape(v)
	case "htmldecode":
		return html.UnescapeString(v)
	case "htmlencode":
		return html.EscapeString(v)
	case "dateparse":
		return applyDateparse(v, f.Args)
	case "timeago", "reltime":
		return applyTimeago(v)
	case "fuzzytime":
		return applyFuzzytime(v)
	case "diacritics":
		return applyDiacritics(v)
	case "validfilename":
		return applyValidFilename(v)
	case "jsonjoinarray":
		return applyJSONJoinArray(v, f.Args)
	case "hexdump", "strdump", "validate":
		return v
	default:
		return v
	}
}

// applyRegexp evaluates a regex filter. The upstream shape is one of:
//
//	args: "pattern"                     # extract first capture group
//	args: ["pattern", "$1 $2"]          # replace, expanding capture groups
//
// We implement both; an invalid pattern leaves the value untouched so
// definitions that ship with broken regexes degrade gracefully.
func applyRegexp(v string, args any) string {
	switch a := args.(type) {
	case string:
		re, err := regexp.Compile(a)
		if err != nil {
			return v
		}
		m := re.FindStringSubmatch(v)
		if len(m) >= 2 {
			return m[1]
		}
		if len(m) == 1 {
			return m[0]
		}
		return ""
	case []any:
		strs := stringSliceArg(a)
		if len(strs) >= 2 {
			re, err := regexp.Compile(strs[0])
			if err != nil {
				return v
			}
			return re.ReplaceAllString(v, strs[1])
		}
		if len(strs) == 1 {
			re, err := regexp.Compile(strs[0])
			if err != nil {
				return v
			}
			m := re.FindStringSubmatch(v)
			if len(m) >= 2 {
				return m[1]
			}
		}
	}
	return v
}

// applySplit returns one piece from "a,b,c" given args [",", 1].
// Negative indices count from the end so `[",", -1]` grabs the last
// piece — handy for stripping a category icon URL down to the slug.
func applySplit(v string, args any) string {
	parts := stringSliceArg(args)
	if len(parts) < 2 {
		return v
	}
	sep := parts[0]
	idx := 0
	if n, err := atoiSafe(parts[1]); err == nil {
		idx = n
	}
	pieces := strings.Split(v, sep)
	if len(pieces) == 0 {
		return v
	}
	if idx < 0 {
		idx = len(pieces) + idx
	}
	if idx < 0 || idx >= len(pieces) {
		return v
	}
	return pieces[idx]
}

// queryParam extracts the named query param from a URL. The empty
// string is returned for malformed URLs and missing params.
func queryParam(v, name string) string {
	if name == "" {
		return v
	}
	u, err := url.Parse(v)
	if err != nil {
		return ""
	}
	return u.Query().Get(name)
}

// stringArg coerces a YAML scalar into a string. Slices return the
// first element so YAML files that quote a single arg as a list do
// not need a different code path.
func stringArg(a any) string {
	switch v := a.(type) {
	case string:
		return v
	case int:
		return atoaSafe(int64(v))
	case int64:
		return atoaSafe(v)
	case []any:
		if len(v) == 0 {
			return ""
		}
		return stringArg(v[0])
	}
	return ""
}

// stringSliceArg coerces a YAML array into a slice of strings. A
// scalar argument round-trips as a 1-element slice so callers can
// handle both shapes uniformly.
func stringSliceArg(a any) []string {
	switch v := a.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			out = append(out, stringArg(x))
		}
		return out
	case string:
		return []string{v}
	}
	return nil
}

// atoiSafe parses an integer without panicking on bad input.
func atoiSafe(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errEmpty
	}
	sign := 1
	if strings.HasPrefix(s, "-") {
		sign = -1
		s = s[1:]
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errBadInt
		}
		n = n*10 + int(c-'0')
	}
	return n * sign, nil
}

// atoaSafe is itoa without strconv import. Inlined so the filter
// helpers stay in one file.
func atoaSafe(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// --- .NET-to-Go date format conversion for dateparse ---

// dotnetToGo maps .NET date format specifiers to Go layout tokens.
// Ordered from longest to shortest to prevent partial matches.
var dotnetFormatPairs = []struct{ from, to string }{
	{"yyyy", "2006"}, {"yy", "06"},
	{"MMMM", "January"}, {"MMM", "Jan"}, {"MM", "01"}, {"M", "1"},
	{"dddd", "Monday"}, {"ddd", "Mon"}, {"dd", "02"}, {"d", "2"},
	{"HH", "15"}, {"hh", "03"}, {"H", "15"}, {"h", "3"},
	{"mm", "04"}, {"m", "4"},
	{"ss", "05"}, {"s", "5"},
	{"tt", "PM"}, {"t", "PM"},
	{"fff", ".000"}, {"ff", ".00"}, {"f", ".0"},
	{"zzz", "-07:00"}, {"zz", "-07"}, {"z", "-7"},
}

func dotnetToGoLayout(format string) string {
	result := format
	for _, p := range dotnetFormatPairs {
		result = strings.ReplaceAll(result, p.from, p.to)
	}
	return result
}

func applyDateparse(v string, args any) string {
	format := stringArg(args)
	if format == "" {
		return v
	}
	layout := dotnetToGoLayout(format)
	t, err := time.Parse(layout, strings.TrimSpace(v))
	if err != nil {
		return v
	}
	return fmt.Sprintf("%d", t.Unix())
}

// --- timeago: "2 hours ago", "just now" ---

var timeagoRe = regexp.MustCompile(`(?i)(\d+)\s*(second|minute|hour|day|week|month|year)s?\s*ago`)
var timeagoNowRe = regexp.MustCompile(`(?i)^(just\s+now|now)$`)

func applyTimeago(v string) string {
	trimmed := strings.TrimSpace(v)
	if timeagoNowRe.MatchString(trimmed) {
		return fmt.Sprintf("%d", time.Now().Unix())
	}
	m := timeagoRe.FindStringSubmatch(trimmed)
	if m == nil {
		return ""
	}
	n, err := atoiSafe(m[1])
	if err != nil {
		return ""
	}
	var d time.Duration
	switch strings.ToLower(m[2]) {
	case "second":
		d = time.Duration(n) * time.Second
	case "minute":
		d = time.Duration(n) * time.Minute
	case "hour":
		d = time.Duration(n) * time.Hour
	case "day":
		d = time.Duration(n) * 24 * time.Hour
	case "week":
		d = time.Duration(n) * 7 * 24 * time.Hour
	case "month":
		d = time.Duration(n) * 30 * 24 * time.Hour
	case "year":
		d = time.Duration(n) * 365 * 24 * time.Hour
	default:
		return ""
	}
	return fmt.Sprintf("%d", time.Now().Add(-d).Unix())
}

// --- fuzzytime: timeago + "today HH:MM", "yesterday HH:MM" ---

var fuzzytimeTodayRe = regexp.MustCompile(`(?i)^today\s+(\d{1,2}):(\d{2})`)
var fuzzytimeYesterdayRe = regexp.MustCompile(`(?i)^yesterday\s+(\d{1,2}):(\d{2})`)

func applyFuzzytime(v string) string {
	trimmed := strings.TrimSpace(v)
	now := time.Now()

	if m := fuzzytimeTodayRe.FindStringSubmatch(trimmed); m != nil {
		h, _ := atoiSafe(m[1])
		min, _ := atoiSafe(m[2])
		t := time.Date(now.Year(), now.Month(), now.Day(), h, min, 0, 0, now.Location())
		return fmt.Sprintf("%d", t.Unix())
	}
	if m := fuzzytimeYesterdayRe.FindStringSubmatch(trimmed); m != nil {
		h, _ := atoiSafe(m[1])
		min, _ := atoiSafe(m[2])
		y := now.AddDate(0, 0, -1)
		t := time.Date(y.Year(), y.Month(), y.Day(), h, min, 0, 0, now.Location())
		return fmt.Sprintf("%d", t.Unix())
	}
	// Fall back to timeago parsing.
	return applyTimeago(trimmed)
}

// --- diacritics: remove combining marks (é→e, ü→u) ---

func applyDiacritics(v string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	result, _, err := transform.String(t, v)
	if err != nil {
		return v
	}
	return result
}

// --- validfilename: strip invalid filename characters ---

var invalidFilenameRe = regexp.MustCompile(`[<>:"/\\|?*]`)

func applyValidFilename(v string) string {
	return invalidFilenameRe.ReplaceAllString(v, "")
}

// --- jsonjoinarray: parse JSON array, join with separator ---

func applyJSONJoinArray(v string, args any) string {
	sep := stringArg(args)
	if sep == "" {
		sep = ", "
	}
	var arr []any
	if err := json.Unmarshal([]byte(v), &arr); err != nil {
		return v
	}
	strs := make([]string, 0, len(arr))
	for _, item := range arr {
		strs = append(strs, fmt.Sprintf("%v", item))
	}
	return strings.Join(strs, sep)
}

var (
	errEmpty  = stringErr("empty")
	errBadInt = stringErr("not an integer")
)

type stringErr string

func (e stringErr) Error() string { return string(e) }
