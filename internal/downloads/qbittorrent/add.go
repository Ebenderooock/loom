package qbittorrent

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/ebenderooock/loom/internal/downloads"
)

// Add submits a magnet, .torrent URL, or raw .torrent payload to the
// running qBittorrent server. The infohash returned by qBittorrent is
// computed client-side from the input because /api/v2/torrents/add
// does not echo it back in v4/v5 — the server replies with a plain
// "Ok." body.
//
// Precedence on the AddRequest follows downloads.AddRequest's
// contract: RawBytes wins, then Magnet, then TorrentURL.
func (c *Client) Add(ctx context.Context, req downloads.AddRequest) (downloads.AddResult, error) {
	body, contentType, hash, err := buildAddBody(req)
	if err != nil {
		return downloads.AddResult{}, err
	}
	if hash == "" {
		// We could not derive the hash up-front (e.g. raw bytes
		// path that did not parse). Fall back to leaving the
		// item id empty; callers can still match by title.
	}

	// Login once before posting so qBittorrent associates the
	// add with our session. /torrents/add tolerates anonymous
	// requests on some configs but most operators require auth.
	if c.cfg.username != "" {
		if err := c.ensureLoggedIn(ctx); err != nil {
			return downloads.AddResult{}, err
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.endpoint("torrents/add"), bytes.NewReader(body))
	if err != nil {
		return downloads.AddResult{}, fmt.Errorf("qbittorrent: building add request: %w", err)
	}
	httpReq.Header.Set("Content-Type", contentType)
	// Preserve body across re-login retries.
	httpReqGetBody := func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	httpReq.GetBody = httpReqGetBody

	respBody, err := c.do(ctx, httpReq)
	if err != nil {
		return downloads.AddResult{}, err
	}
	if !addLooksOK(respBody) {
		return downloads.AddResult{}, fmt.Errorf("%w: torrents/add rejected the input: %q",
			ErrServer, strings.TrimSpace(string(respBody)))
	}
	return downloads.AddResult{
		ClientID: c.id,
		ItemID:   hash,
	}, nil
}

// ensureLoggedIn does a best-effort login. Subsequent calls succeed
// even if the cookie is already valid; qBittorrent will simply issue
// a new SID.
func (c *Client) ensureLoggedIn(ctx context.Context) error {
	// Cheap heuristic: if the cookie jar already has any cookie
	// for our base URL, skip. The do() layer transparently
	// refreshes on 403 anyway, so this is purely an optimisation.
	if c.http.Jar != nil {
		if cs := c.http.Jar.Cookies(c.cfg.baseURL); len(cs) > 0 {
			return nil
		}
	}
	return c.login(ctx)
}

// addLooksOK applies qBittorrent's flexible success contract: a
// trimmed body of "Ok." or an empty 200 are both treated as success.
func addLooksOK(body []byte) bool {
	t := strings.TrimSpace(string(body))
	return t == "" || strings.EqualFold(t, "Ok.")
}

// buildAddBody assembles the multipart payload for /torrents/add and
// returns the body, content type, and (best-effort) infohash. The
// body is buffered in full so it is replayable on a re-login retry.
func buildAddBody(req downloads.AddRequest) ([]byte, string, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	hash := ""

	switch {
	case len(req.RawBytes) > 0:
		fw, err := w.CreateFormFile("torrents", filenameFor(req))
		if err != nil {
			return nil, "", "", fmt.Errorf("qbittorrent: form file: %w", err)
		}
		if _, err := fw.Write(req.RawBytes); err != nil {
			return nil, "", "", fmt.Errorf("qbittorrent: writing torrent bytes: %w", err)
		}
		if h, ok := infohashFromTorrent(req.RawBytes); ok {
			hash = h
		}
	case req.Magnet != "":
		if err := w.WriteField("urls", req.Magnet); err != nil {
			return nil, "", "", err
		}
		hash = infohashFromMagnet(req.Magnet)
	case req.TorrentURL != "":
		if err := w.WriteField("urls", req.TorrentURL); err != nil {
			return nil, "", "", err
		}
	default:
		return nil, "", "", fmt.Errorf("qbittorrent: AddRequest has neither RawBytes, Magnet, nor TorrentURL")
	}

	if req.Category != "" {
		_ = w.WriteField("category", req.Category)
	}
	if req.SavePath != "" {
		_ = w.WriteField("savepath", req.SavePath)
	}
	if len(req.Tags) > 0 {
		_ = w.WriteField("tags", strings.Join(req.Tags, ","))
	}
	if req.Title != "" {
		// qBittorrent honours rename only on multi-file torrents
		// at add time, but passing it for single-file is harmless.
		_ = w.WriteField("rename", req.Title)
	}
	// "paused" is a string-bool. We always send "false" to mirror
	// the Loom default (start immediately) — operators that want
	// a paused-on-add behaviour can set it via post-Add Pause().
	_ = w.WriteField("paused", "false")

	if err := w.Close(); err != nil {
		return nil, "", "", fmt.Errorf("qbittorrent: closing multipart writer: %w", err)
	}
	return buf.Bytes(), w.FormDataContentType(), hash, nil
}

// filenameFor picks a stable filename for the multipart "torrents"
// part. qBittorrent does not parse the filename, but a meaningful
// value helps when capturing traffic.
func filenameFor(req downloads.AddRequest) string {
	name := req.Title
	if name == "" {
		name = "loom-upload"
	}
	if !strings.HasSuffix(strings.ToLower(name), ".torrent") {
		name += ".torrent"
	}
	return name
}

// infohashFromMagnet extracts the BTIH (BitTorrent v1 infohash) from
// a magnet URI's xt parameter. Returns "" if the URI does not carry a
// recognisable hash.
func infohashFromMagnet(magnet string) string {
	u, err := url.Parse(magnet)
	if err != nil {
		return ""
	}
	for _, xt := range u.Query()["xt"] {
		const prefix = "urn:btih:"
		if strings.HasPrefix(xt, prefix) {
			return strings.ToLower(strings.TrimPrefix(xt, prefix))
		}
	}
	return ""
}

// infohashFromTorrent computes the SHA-1 of the bencoded `info`
// dictionary, i.e. the canonical BTIH for a v1 torrent file. The
// parser is intentionally minimal — we only need to find the `info`
// dict's byte range and hash it. Returns ("", false) if parsing
// fails; the caller treats that as "no hash known" rather than an
// error, since qBittorrent will accept the file regardless.
func infohashFromTorrent(b []byte) (string, bool) {
	idx := bytes.Index(b, []byte("4:infod"))
	if idx < 0 {
		return "", false
	}
	start := idx + len("4:info")
	end, ok := bencodeEnd(b, start)
	if !ok {
		return "", false
	}
	sum := sha1.Sum(b[start:end])
	return hex.EncodeToString(sum[:]), true
}

// bencodeEnd locates the byte index immediately after the bencoded
// value beginning at b[start]. Recursive descent is fine here —
// torrent metainfo is shallow and bounded.
func bencodeEnd(b []byte, start int) (int, bool) {
	if start >= len(b) {
		return 0, false
	}
	switch c := b[start]; {
	case c == 'd' || c == 'l':
		i := start + 1
		for i < len(b) && b[i] != 'e' {
			next, ok := bencodeEnd(b, i)
			if !ok {
				return 0, false
			}
			i = next
		}
		if i >= len(b) {
			return 0, false
		}
		return i + 1, true
	case c == 'i':
		j := bytes.IndexByte(b[start:], 'e')
		if j < 0 {
			return 0, false
		}
		return start + j + 1, true
	case c >= '0' && c <= '9':
		colon := bytes.IndexByte(b[start:], ':')
		if colon < 0 {
			return 0, false
		}
		n, err := strconv.Atoi(string(b[start : start+colon]))
		if err != nil || n < 0 {
			return 0, false
		}
		return start + colon + 1 + n, true
	default:
		return 0, false
	}
}
