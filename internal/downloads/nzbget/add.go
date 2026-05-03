package nzbget

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/loomctl/loom/internal/downloads"
)

// defaultPriority is the NZBGet integer priority assigned when the
// AddRequest does not specify one. Zero is the NZBGet "normal"
// priority.
const defaultPriority = 0

// appendParams matches the positional parameter list NZBGet expects
// for the JSON-RPC `append` method (NZBGet 17+):
//
//	append(NZBFilename, NZBContent, Category, Priority,
//	       AddToTop, AddPaused, DupeKey, DupeScore,
//	       DupeMode, PostProcessParameters)
//
// Each element corresponds positionally; we marshal a fixed-size
// []any so unknown fields cannot be silently dropped.
func appendParams(filename, content, category string, priority int, addToTop, addPaused bool, dupeKey string, dupeScore int, dupeMode string, postProcess [][]string) []any {
	if dupeMode == "" {
		// "FORCE" lets the caller override; "SCORE" is NZBGet's
		// safe default when DupeScore is non-zero. We pass an empty
		// string through unchanged so NZBGet's own default applies.
		dupeMode = ""
	}
	pp := make([][]string, 0, len(postProcess))
	pp = append(pp, postProcess...)
	return []any{
		filename, content, category, priority,
		addToTop, addPaused, dupeKey, dupeScore,
		dupeMode, pp,
	}
}

// Add submits a job to NZBGet. URL-by-fetch is delegated to NZBGet
// itself by passing the URL as `NZBFilename` with empty
// `NZBContent` (NZBGet 17+ supports this); raw bytes are base64
// encoded into `NZBContent`. Returns the assigned NZBID as the
// per-client item ID.
//
// NZBGet's `append` JSON-RPC method returns a positive int (the
// NZBID) on success, 0 on a deduplicated/rejected add, and -1 on
// validation failure. We map 0 and -1 onto ErrMissingNZBID and
// ErrServer respectively so callers see typed errors.
func (c *Client) Add(ctx context.Context, req downloads.AddRequest) (downloads.AddResult, error) {
	priority, postProcess, dupeKey, dupeScore, dupeMode := readAddTags(req.Tags)

	switch {
	case len(req.RawBytes) > 0:
		return c.appendBytes(ctx, req, priority, postProcess, dupeKey, dupeScore, dupeMode)
	case req.NZBURL != "":
		return c.appendURL(ctx, req, priority, postProcess, dupeKey, dupeScore, dupeMode)
	default:
		return downloads.AddResult{}, fmt.Errorf("%w: AddRequest has neither NZBURL nor RawBytes", ErrMalformedNZB)
	}
}

// appendBytes base64-encodes the NZB body and POSTs it via append().
// Filename is derived from req.Title; NZBGet uses it for the job
// label and falls back to the contained <head> metadata otherwise.
func (c *Client) appendBytes(ctx context.Context, req downloads.AddRequest, priority int, pp [][]string, dupeKey string, dupeScore int, dupeMode string) (downloads.AddResult, error) {
	filename := req.Title
	if filename == "" {
		filename = "upload.nzb"
	}
	if !strings.HasSuffix(strings.ToLower(filename), ".nzb") {
		filename += ".nzb"
	}
	content := base64.StdEncoding.EncodeToString(req.RawBytes)
	addToTop, addPaused := readBoolTags(req.Tags)
	params := appendParams(filename, content, req.Category, priority, addToTop, addPaused, dupeKey, dupeScore, dupeMode, pp)
	return c.invokeAppend(ctx, params)
}

// appendURL hands NZBGet a fetchable NZB URL. NZBGet 17+ recognises
// the URL form: pass the URL as NZBFilename with empty NZBContent.
func (c *Client) appendURL(ctx context.Context, req downloads.AddRequest, priority int, pp [][]string, dupeKey string, dupeScore int, dupeMode string) (downloads.AddResult, error) {
	addToTop, addPaused := readBoolTags(req.Tags)
	params := appendParams(req.NZBURL, "", req.Category, priority, addToTop, addPaused, dupeKey, dupeScore, dupeMode, pp)
	return c.invokeAppend(ctx, params)
}

// invokeAppend runs the JSON-RPC call and converts the integer
// result into a typed AddResult.
func (c *Client) invokeAppend(ctx context.Context, params []any) (downloads.AddResult, error) {
	var nzbID int64
	if err := c.call(ctx, "append", params, &nzbID); err != nil {
		return downloads.AddResult{}, err
	}
	switch {
	case nzbID > 0:
		return downloads.AddResult{ClientID: c.id, ItemID: strconv.FormatInt(nzbID, 10)}, nil
	case nzbID == 0:
		// NZBGet returned "deduplicated/rejected" — there is no
		// NZBID to track. Surface ErrMissingNZBID so the caller
		// distinguishes it from a transport failure.
		return downloads.AddResult{}, fmt.Errorf("%w: NZBGet returned 0", ErrMissingNZBID)
	default:
		return downloads.AddResult{}, fmt.Errorf("%w: append returned %d", ErrServer, nzbID)
	}
}

// readAddTags pulls the NZBGet-specific knobs out of the
// AddRequest.Tags slice. The recognised prefixes mirror the JSON-RPC
// field names so the convention is greppable across logs.
//
// Recognised tags:
//
//	priority=<int>          NZBGet integer priority
//	pp_<name>=<value>       PostProcessParameters key=value pair
//	dupekey=<string>        DupeKey for deduplication
//	dupescore=<int>         DupeScore for deduplication
//	dupemode=<string>       DupeMode (SCORE | ALL | FORCE)
func readAddTags(tags []string) (priority int, pp [][]string, dupeKey string, dupeScore int, dupeMode string) {
	priority = defaultPriority
	for _, tag := range tags {
		switch {
		case strings.HasPrefix(tag, "priority="):
			if v, err := strconv.Atoi(strings.TrimPrefix(tag, "priority=")); err == nil {
				priority = v
			}
		case strings.HasPrefix(tag, "pp_"):
			rest := strings.TrimPrefix(tag, "pp_")
			if eq := strings.IndexByte(rest, '='); eq > 0 {
				pp = append(pp, []string{rest[:eq], rest[eq+1:]})
			}
		case strings.HasPrefix(tag, "dupekey="):
			dupeKey = strings.TrimPrefix(tag, "dupekey=")
		case strings.HasPrefix(tag, "dupescore="):
			if v, err := strconv.Atoi(strings.TrimPrefix(tag, "dupescore=")); err == nil {
				dupeScore = v
			}
		case strings.HasPrefix(tag, "dupemode="):
			dupeMode = strings.TrimPrefix(tag, "dupemode=")
		}
	}
	return priority, pp, dupeKey, dupeScore, dupeMode
}

// readBoolTags extracts AddToTop / AddPaused booleans. Kept
// separate from readAddTags because both append paths need them
// regardless of the priority/dedupe block.
func readBoolTags(tags []string) (addToTop, addPaused bool) {
	for _, tag := range tags {
		switch tag {
		case "add_to_top":
			addToTop = true
		case "add_paused":
			addPaused = true
		}
	}
	return addToTop, addPaused
}
