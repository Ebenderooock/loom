package cardigann

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ebenderooock/loom/internal/indexers"
)

// extractRowsJSON parses a JSON response body and extracts result rows
// using dot-path selectors from the Cardigann definition. This handles
// indexers like YTS and The Pirate Bay that return JSON instead of HTML.
//
// The key Cardigann JSON concepts:
//
//   - rows.selector: a dot-path like "data.movies" navigating the JSON
//   - rows.attribute: a nested array key (e.g. "torrents") — each entry
//     in the parent row is expanded with this sub-array
//   - rows.multiple: when true, each attribute entry becomes a separate
//     result, inheriting parent fields via ".." selectors
//   - field selectors: dot-paths relative to the current row object;
//     ".." prefix means "look in the parent object"
func (e *Engine) extractRowsJSON(body []byte, tctx templateContext) ([]indexers.Result, error) {
	var root any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("cardigann: parse search json: %w", err)
	}

	rowSelector := e.def.Search.Rows.Selector
	if strings.Contains(rowSelector, "{{") {
		expanded, terr := e.expandTemplate(rowSelector, tctx)
		if terr != nil {
			return nil, fmt.Errorf("cardigann: row selector template: %w", terr)
		}
		rowSelector = expanded
	}

	// Navigate to the row array using the dot-path selector.
	// Handle "$" prefix (Prowlarr JSONPath root notation).
	rowSelector = strings.TrimPrefix(rowSelector, "$")
	rowSelector = strings.TrimPrefix(rowSelector, ".")

	var rowsRaw any
	if rowSelector == "" {
		// Empty selector means the root IS the array.
		rowsRaw = root
	} else {
		rowsRaw = jsonNavigate(root, rowSelector)
	}
	rowsArr, ok := toSlice(rowsRaw)
	if !ok {
		slog.Debug("cardigann: json rows selector returned non-array or nil",
			"indexer", e.id, "selector", rowSelector)
		return nil, nil
	}

	slog.Debug("cardigann: extractRowsJSON",
		"indexer", e.id, "rowSelector", rowSelector, "matchedRows", len(rowsArr))

	attr := e.def.Search.Rows.Attribute
	multiple := e.def.Search.Rows.Multiple
	missingAttrOK := e.def.Search.Rows.MissingAttributeEqualsNoResults

	// Build a flat list of (parent, child) JSON objects. When there is
	// no attribute expansion, child == parent and parent is nil.
	type rowPair struct {
		parent map[string]any // nil when no attribute expansion
		child  map[string]any
	}
	var pairs []rowPair

	for _, raw := range rowsArr {
		rowObj, isMap := raw.(map[string]any)
		if !isMap {
			continue
		}

		if attr == "" || !multiple {
			pairs = append(pairs, rowPair{child: rowObj})
			continue
		}

		// Attribute expansion: e.g. each movie has a "torrents" array.
		subRaw, hasAttr := rowObj[attr]
		if !hasAttr {
			if missingAttrOK {
				continue
			}
			slog.Debug("cardigann: json row missing attribute",
				"indexer", e.id, "attribute", attr)
			continue
		}
		subArr, subOK := toSlice(subRaw)
		if !subOK || len(subArr) == 0 {
			if missingAttrOK {
				continue
			}
			continue
		}
		for _, sub := range subArr {
			subObj, isSubMap := sub.(map[string]any)
			if !isSubMap {
				continue
			}
			pairs = append(pairs, rowPair{parent: rowObj, child: subObj})
		}
	}

	// Extract fields from each pair.
	out := make([]indexers.Result, 0, len(pairs))
	for _, p := range pairs {
		r, ok := e.extractOneJSON(p.child, p.parent, tctx)
		if !ok {
			continue
		}
		r.IndexerID = e.id
		out = append(out, r)
	}
	return out, nil
}

// extractOneJSON maps Cardigann field definitions against a JSON object.
// parentObj is non-nil when attribute expansion is active and provides
// values for ".." (parent-access) selectors.
func (e *Engine) extractOneJSON(obj, parentObj map[string]any, tctx templateContext) (indexers.Result, bool) {
	r := indexers.Result{}
	values := map[string]string{}

	fieldCtx := tctx
	fieldCtx.Result = values

	// Sort field names once for deterministic iteration across all phases.
	fieldNames := sortedFieldNames(e.def.Search.Fields)

	// Phase 1 — selector-based extraction from JSON keys.
	for _, name := range fieldNames {
		field := e.def.Search.Fields[name]
		if field.Selector == "" {
			continue
		}
		sel := field.Selector
		if strings.Contains(sel, "{{") {
			var err error
			sel, err = e.expandTemplate(sel, fieldCtx)
			if err != nil {
				slog.Warn("cardigann: json field selector template error",
					"indexer", e.id, "field", name, "err", err)
				continue
			}
		}

		v := jsonFieldValue(sel, obj, parentObj)

		// Case mapping (like HTML mode).
		if len(field.Case) > 0 {
			v = applyCaseMap(v, field.Case)
		}

		v = applyFilters(v, e.expandFilterArgs(field.Filters, fieldCtx))
		values[name] = v
	}

	// Phase 2 — text-template fields (identical to HTML mode).
	maxPasses := 0
	for _, name := range fieldNames {
		field := e.def.Search.Fields[name]
		if field.Selector == "" && field.Text != "" {
			maxPasses++
		}
	}
	for pass := 0; pass <= maxPasses; pass++ {
		changed := false
		for _, name := range fieldNames {
			field := e.def.Search.Fields[name]
			if field.Selector != "" || field.Text == "" {
				continue
			}
			expanded := field.Text
			if strings.Contains(expanded, "{{") {
				var terr error
				expanded, terr = e.expandTemplate(expanded, fieldCtx)
				if terr != nil {
					continue
				}
			}
			next := applyFilters(expanded, e.expandFilterArgs(field.Filters, fieldCtx))
			if values[name] != next {
				values[name] = next
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	// Warn if fixed-point iteration did not converge.
	{
		countUnresolved := 0
		for _, name := range fieldNames {
			field := e.def.Search.Fields[name]
			if field.Selector != "" || field.Text == "" {
				continue
			}
			expanded := field.Text
			if strings.Contains(expanded, "{{") {
				var terr error
				expanded, terr = e.expandTemplate(expanded, fieldCtx)
				if terr != nil {
					continue
				}
			}
			next := applyFilters(expanded, e.expandFilterArgs(field.Filters, fieldCtx))
			if values[name] != next {
				countUnresolved++
			}
		}
		if countUnresolved > 0 {
			slog.Warn("cardigann: json fixed-point iteration did not converge",
				"indexer", e.id,
				"passes", maxPasses,
				"remaining_unresolved", countUnresolved)
		}
	}

	// Phase 3 — default fallbacks.
	for _, name := range fieldNames {
		field := e.def.Search.Fields[name]
		if field.Default == "" || values[name] != "" {
			continue
		}
		fallback := field.Default
		if strings.Contains(fallback, "{{") {
			var terr error
			fallback, terr = e.expandTemplate(fallback, fieldCtx)
			if terr != nil {
				continue
			}
		}
		values[name] = applyFilters(fallback, e.expandFilterArgs(field.Filters, fieldCtx))
	}

	// Map field values to Result struct (same as HTML mode).
	r.Title = strings.TrimSpace(values["title"])
	r.GUID = strings.TrimSpace(values["guid"])
	r.Link = strings.TrimSpace(values["download"])
	if r.Link == "" {
		r.Link = strings.TrimSpace(values["link"])
	}
	r.InfoURL = strings.TrimSpace(values["details"])
	if r.InfoURL == "" {
		r.InfoURL = strings.TrimSpace(values["comments"])
	}
	r.Quality = strings.TrimSpace(values["quality"])
	r.MagnetURI = strings.TrimSpace(values["magnet"])
	r.Infohash = strings.TrimSpace(values["infohash"])
	if size := values["size"]; size != "" {
		r.Size = parseSize(size)
	}
	if pd := values["date"]; pd != "" {
		r.PubDate = parseDateBestEffort(pd)
	}
	if seedersStr, ok := values["seeders"]; ok && seedersStr != "" {
		v := parseInt(seedersStr)
		r.Seeders = &v
	}
	if peersStr, ok := values["peers"]; ok && peersStr != "" {
		v := parseInt(peersStr)
		r.Peers = &v
	} else if leechersStr, ok := values["leechers"]; ok && leechersStr != "" && r.Seeders != nil {
		v := *r.Seeders + parseInt(leechersStr)
		r.Peers = &v
	}
	if cat := values["category"]; cat != "" {
		r.Category = e.mapSiteCategory(cat)
	}
	if dvf := values["downloadvolumefactor"]; dvf != "" {
		r.Freeleech = dvf == "0" || dvf == "0.0"
	}
	if iv := values["_internal"]; iv != "" && iv != "0" {
		r.Internal = true
	}
	if sv := values["_scene"]; sv != "" && sv != "0" {
		r.Scene = true
	}
	r.Link = e.absoluteURL(r.Link)
	r.InfoURL = e.absoluteURL(r.InfoURL)

	if r.Title == "" || (r.Link == "" && r.MagnetURI == "" && r.Infohash == "") {
		return r, false
	}
	return r, true
}

// jsonNavigate walks a dot-separated path through a JSON value.
// e.g. "data.movies" navigates root["data"]["movies"].
func jsonNavigate(root any, path string) any {
	if path == "" {
		return root
	}
	parts := strings.Split(path, ".")
	current := root
	for _, part := range parts {
		if part == "" {
			continue
		}
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = m[part]
	}
	return current
}

// jsonFieldValue extracts a string value for a field selector from a
// JSON object. Supports ".." prefix for parent-access (e.g. "..year"
// reads the parent object's "year" key).
func jsonFieldValue(sel string, obj, parentObj map[string]any) string {
	// ".." prefix means look in parent object
	if strings.HasPrefix(sel, "..") {
		key := sel[2:]
		if parentObj != nil {
			return jsonValueToString(jsonNavigate(parentObj, key))
		}
		// Fall back to current object if no parent
		return jsonValueToString(jsonNavigate(obj, key))
	}
	return jsonValueToString(jsonNavigate(obj, sel))
}

// jsonValueToString converts a JSON value to its string representation.
func jsonValueToString(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		// Avoid scientific notation; use integer format when possible
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	case json.Number:
		return val.String()
	default:
		// For arrays/objects, marshal back to JSON string
		b, err := json.Marshal(val)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

// toSlice attempts to convert a JSON value to []any.
func toSlice(v any) ([]any, bool) {
	if v == nil {
		return nil, false
	}
	arr, ok := v.([]any)
	return arr, ok
}

// applyCaseMap maps a value through a case map. The "*" key is the
// default fallback.
func applyCaseMap(val string, caseMap map[string]string) string {
	if mapped, ok := caseMap[val]; ok {
		return mapped
	}
	if def, ok := caseMap["*"]; ok {
		return def
	}
	return val
}
