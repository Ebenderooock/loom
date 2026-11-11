package appearance

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// 1x1 lossless WebP.
const tinyWebP = "UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA=="

func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 200, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func testJPEG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 3)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// pngWithHeaderSize rewrites the IHDR dimensions (with a valid CRC) so the
// dimension checks can be exercised without allocating huge images.
func pngWithHeaderSize(t *testing.T, w, h uint32) []byte {
	t.Helper()
	data := testPNG(t, 1, 1)
	// 8-byte signature, 4-byte length, then "IHDR" at 12..16, data 16..29, CRC 29..33.
	binary.BigEndian.PutUint32(data[16:], w)
	binary.BigEndian.PutUint32(data[20:], h)
	binary.BigEndian.PutUint32(data[29:], crc32.ChecksumIEEE(data[12:29]))
	return data
}

func TestValidateBackgroundImage(t *testing.T) {
	webp, err := base64.StdEncoding.DecodeString(tinyWebP)
	if err != nil {
		t.Fatal(err)
	}
	pngData := testPNG(t, 3, 2)
	accepted := []struct {
		name, contentType string
		data              []byte
		w, h              int
	}{
		{"png", "image/png", pngData, 3, 2},
		{"jpeg", "image/jpeg", testJPEG(t), 4, 3},
		{"webp", "image/webp", webp, 1, 1},
		{"max side", "image/png", pngWithHeaderSize(t, MaxBackgroundSide, 1), MaxBackgroundSide, 1},
	}
	for _, tt := range accepted {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "max side" {
				// Header-only rewrite cannot fully decode; check the config gate only.
				_, err := ValidateBackgroundImage(tt.data)
				var imgErr *ImageError
				if !errors.As(err, &imgErr) || strings.Contains(imgErr.Message, "dimensions") {
					t.Fatalf("max side rejected by dimension check: %v", err)
				}
				return
			}
			img, err := ValidateBackgroundImage(tt.data)
			if err != nil {
				t.Fatal(err)
			}
			if img.ContentType != tt.contentType || img.Width != tt.w || img.Height != tt.h || img.Size != len(tt.data) || len(img.SHA256) != 64 {
				t.Fatalf("unexpected metadata: %+v", img)
			}
			if _, ok := InternalBackground(img.URL()); !ok {
				t.Fatalf("URL %q is not an internal background URL", img.URL())
			}
		})
	}
	rejected := []struct {
		name   string
		data   []byte
		status int
	}{
		{"empty", nil, 400},
		{"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), 415},
		{"svg with xml prolog", []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"/>`), 415},
		{"gif", []byte("GIF89a\x01\x00\x01\x00\x00\x00\x00;"), 415},
		{"html", []byte("<html><script>alert(1)</script></html>"), 415},
		{"text", []byte("hello"), 415},
		{"truncated png", pngData[:len(pngData)-20], 400},
		{"corrupt png header", append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 64)...), 400},
		{"too wide", pngWithHeaderSize(t, MaxBackgroundSide+1, 1), 400},
		{"too tall", pngWithHeaderSize(t, 1, MaxBackgroundSide+1), 400},
		{"too many pixels", pngWithHeaderSize(t, 7680, 4321), 400},
		{"too large", append(append([]byte{}, pngData...), make([]byte, MaxBackgroundBytes)...), 413},
	}
	for _, tt := range rejected {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateBackgroundImage(tt.data)
			var imgErr *ImageError
			if !errors.As(err, &imgErr) || imgErr.Status != tt.status {
				t.Fatalf("err = %v; want status %d", err, tt.status)
			}
		})
	}
}

func multipartBody(t *testing.T, fields map[string][][]byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for name, values := range fields {
		for _, v := range values {
			// Declared filename/type are deliberately misleading; the server sniffs.
			part, err := mw.CreateFormFile(name, "evil.svg")
			if err != nil {
				t.Fatal(err)
			}
			_, _ = part.Write(v)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, mw.FormDataContentType()
}

func upload(t *testing.T, router http.Handler, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	body, ct := multipartBody(t, map[string][][]byte{"image": {data}})
	r := httptest.NewRequest(http.MethodPost, "/background", body)
	r.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func uploadURL(t *testing.T, router http.Handler, data []byte) string {
	t.Helper()
	w := upload(t, router, data)
	var got struct{ URL string }
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &got) != nil {
		t.Fatalf("upload = %d, %s", w.Code, w.Body.String())
	}
	return got.URL
}

func get(router http.Handler, target string, headers ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, target, nil)
	for i := 0; i+1 < len(headers); i += 2 {
		r.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func putSettings(router http.Handler, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body)))
	return w
}

func TestBackgroundUploadServeAndSaveLifecycle(t *testing.T) {
	store := testStore(t)
	router := Router(store, nil, nil)
	ctx := context.Background()

	if w := get(router, "/background"); w.Code != http.StatusNotFound {
		t.Fatalf("missing GET = %d", w.Code)
	}
	first := testPNG(t, 2, 2)
	firstURL := uploadURL(t, router, first)
	firstHash, ok := InternalBackground(firstURL)
	if !ok || firstHash == "" || !strings.HasPrefix(firstURL, "/api/v1/appearance/background?v=") {
		t.Fatalf("unexpected upload URL %q", firstURL)
	}
	// Upload alone does not change saved settings.
	if got, err := store.Get(ctx); err != nil || !reflect.DeepEqual(got, Defaults()) {
		t.Fatalf("upload changed settings: %+v, %v", got, err)
	}

	w := get(router, "/background?v="+firstHash)
	if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), first) {
		t.Fatalf("GET staged = %d", w.Code)
	}
	h := w.Header()
	if h.Get("Content-Type") != "image/png" || h.Get("X-Content-Type-Options") != "nosniff" ||
		h.Get("ETag") != `"`+firstHash+`"` || !strings.Contains(h.Get("Cache-Control"), "immutable") ||
		!strings.HasPrefix(h.Get("Cache-Control"), "private") {
		t.Fatalf("unexpected headers: %v", h)
	}
	if w := get(router, "/background?v="+firstHash, "If-None-Match", `W/"x", "`+firstHash+`"`); w.Code != http.StatusNotModified || w.Body.Len() != 0 {
		t.Fatalf("conditional GET = %d", w.Code)
	}
	// The bare path serves only the saved image; a pending upload is not visible.
	if w := get(router, "/background"); w.Code != http.StatusNotFound {
		t.Fatalf("bare GET with only a pending upload = %d", w.Code)
	}
	if w := get(router, "/background?v="+strings.Repeat("0", 64)); w.Code != http.StatusNotFound {
		t.Fatalf("unknown hash GET = %d", w.Code)
	}
	for _, bad := range []string{"abc", strings.Repeat("A", 64), "../../etc"} {
		if w := get(router, "/background?v="+bad); w.Code != http.StatusBadRequest {
			t.Fatalf("invalid hash %q GET = %d", bad, w.Code)
		}
	}

	settings := `{"theme":"dark","accent":"default","backgroundUrl":"` + firstURL + `","backgroundDim":30}`
	if w := putSettings(router, settings); w.Code != http.StatusOK {
		t.Fatalf("save = %d, %s", w.Code, w.Body.String())
	}
	if w := get(router, "/background", "If-None-Match", `"`+firstHash+`"`); w.Code != http.StatusNotModified {
		t.Fatalf("bare conditional GET = %d", w.Code)
	} else if w.Header().Get("Cache-Control") != "private, no-cache" {
		t.Fatalf("bare path cache-control = %q", w.Header().Get("Cache-Control"))
	}

	// A newer upload is staged; the saved background keeps serving.
	jpg := testJPEG(t)
	secondURL := uploadURL(t, router, jpg)
	secondHash, _ := InternalBackground(secondURL)
	if w := get(router, "/background"); !bytes.Equal(w.Body.Bytes(), first) {
		t.Fatal("bare path should serve the saved (active) image")
	}
	if w := get(router, "/background?v="+firstHash); w.Code != http.StatusOK {
		t.Fatalf("active hash GET = %d", w.Code)
	}
	if w := get(router, "/background?v="+secondHash); w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("staged hash GET = %d %q", w.Code, w.Header().Get("Content-Type"))
	}

	// A third upload replaces the staged slot, so only two images are kept.
	thirdURL := uploadURL(t, router, testPNG(t, 5, 5))
	if w := get(router, "/background?v="+secondHash); w.Code != http.StatusNotFound {
		t.Fatalf("replaced staged image still served: %d", w.Code)
	}
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM schema_meta WHERE key LIKE 'appearance.background.%'`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("stored image rows = %d, %v; want 2", count, err)
	}
	if w := putSettings(router, strings.Replace(settings, firstURL, secondURL, 1)); w.Code != http.StatusBadRequest ||
		!strings.Contains(w.Body.String(), "upload it again") {
		t.Fatalf("save with discarded image = %d, %s", w.Code, w.Body.String())
	}
	if w := putSettings(router, strings.Replace(settings, firstURL, thirdURL, 1)); w.Code != http.StatusOK {
		t.Fatalf("save third = %d, %s", w.Code, w.Body.String())
	}

	// Restart: a fresh store/router reads everything from the DB.
	restarted := Router(&Store{db: store.db}, nil, nil)
	thirdHash, _ := InternalBackground(thirdURL)
	if w := get(restarted, "/background"); w.Code != http.StatusOK || w.Header().Get("ETag") != `"`+thirdHash+`"` {
		t.Fatalf("after restart GET = %d, etag %q", w.Code, w.Header().Get("ETag"))
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM schema_meta WHERE key LIKE 'appearance.background.%'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("stored image rows after promotion = %d, %v; want 1", count, err)
	}
	// Bare internal path is accepted when an active image exists.
	if w := putSettings(restarted, strings.Replace(settings, firstURL, BackgroundPath, 1)); w.Code != http.StatusOK {
		t.Fatalf("save bare path = %d, %s", w.Code, w.Body.String())
	}
	// Switching to an external URL (or removing the background) retires the upload.
	if w := putSettings(restarted, strings.Replace(settings, firstURL, "https://example.com/bg.jpg", 1)); w.Code != http.StatusOK {
		t.Fatalf("save external = %d, %s", w.Code, w.Body.String())
	}
	if w := get(restarted, "/background?v="+thirdHash); w.Code != http.StatusNotFound {
		t.Fatalf("retired image still served: %d", w.Code)
	}
	if w := putSettings(restarted, strings.Replace(settings, firstURL, thirdURL, 1)); w.Code != http.StatusBadRequest {
		t.Fatalf("save retired image = %d, %s", w.Code, w.Body.String())
	}
}

func TestSaveInternalBackgroundWithoutUploadFails(t *testing.T) {
	store := testStore(t)
	router := Router(store, nil, nil)
	for _, url := range []string{BackgroundPath, BackgroundPath + "?v=" + strings.Repeat("a", 64)} {
		w := putSettings(router, `{"theme":"dark","accent":"default","backgroundUrl":"`+url+`","backgroundDim":30}`)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d", url, w.Code)
		}
	}
	if got, err := store.Get(context.Background()); err != nil || !reflect.DeepEqual(got, Defaults()) {
		t.Fatalf("failed save changed settings: %+v, %v", got, err)
	}
}

func TestBackgroundUploadRejections(t *testing.T) {
	pngData := testPNG(t, 1, 1)
	tests := []struct {
		name   string
		build  func() (io.Reader, string)
		status int
	}{
		{"not multipart", func() (io.Reader, string) { return bytes.NewReader(pngData), "image/png" }, 400},
		{"no fields", func() (io.Reader, string) { return multipartBody(t, nil) }, 400},
		{"wrong field", func() (io.Reader, string) { return multipartBody(t, map[string][][]byte{"file": {pngData}}) }, 400},
		{"two images", func() (io.Reader, string) {
			return multipartBody(t, map[string][][]byte{"image": {pngData, pngData}})
		}, 400},
		{"svg", func() (io.Reader, string) {
			return multipartBody(t, map[string][][]byte{"image": {[]byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)}})
		}, 415},
		{"file over 5 MiB", func() (io.Reader, string) {
			return multipartBody(t, map[string][][]byte{"image": {append(append([]byte{}, pngData...), make([]byte, MaxBackgroundBytes)...)}})
		}, 413},
		{"body over limit", func() (io.Reader, string) {
			// The multipart body itself exceeds the request limit.
			return multipartBody(t, map[string][][]byte{"image": {make([]byte, maxUploadBodyBytes+1)}})
		}, 413},
		{"malformed multipart", func() (io.Reader, string) {
			return strings.NewReader("--x\r\nbroken"), "multipart/form-data; boundary=x"
		}, 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := testStore(t)
			router := Router(store, nil, nil)
			body, ct := tt.build()
			r := httptest.NewRequest(http.MethodPost, "/background", body)
			r.Header.Set("Content-Type", ct)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if w.Code != tt.status || !strings.Contains(w.Body.String(), `"error"`) {
				t.Fatalf("status = %d, want %d; body = %s", w.Code, tt.status, w.Body.String())
			}
			if _, err := store.Background(context.Background(), ""); !errors.Is(err, ErrBackgroundNotFound) {
				t.Fatalf("rejected upload was stored: %v", err)
			}
		})
	}
}

func TestBackgroundStorageFailuresAreLoggedAndReported(t *testing.T) {
	store := testStore(t)
	if err := store.db.Close(); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	router := Router(store, slog.New(slog.NewTextHandler(&logs, nil)), nil)
	for _, w := range []*httptest.ResponseRecorder{get(router, "/background"), upload(t, router, testPNG(t, 1, 1))} {
		if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "database is closed") {
			t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
		}
	}
	if !strings.Contains(logs.String(), "database is closed") || strings.Contains(logs.String(), "iVBOR") {
		t.Fatalf("unexpected logs: %s", logs.String())
	}
}

func TestPaletteRoundTripAndCompatibility(t *testing.T) {
	store := testStore(t)
	router := Router(store, nil, nil)
	ctx := context.Background()

	// Legacy stored settings (pre-palette) load unchanged with no palette.
	if _, err := store.db.Exec(`INSERT INTO schema_meta (key, value) VALUES (?, ?)`, settingsKey,
		`{"theme":"dark","accent":"rose","backgroundUrl":"","backgroundDim":20}`); err != nil {
		t.Fatal(err)
	}
	w := get(router, "/")
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "palette") {
		t.Fatalf("legacy GET = %d, %s", w.Code, w.Body.String())
	}

	withPalette := `{"theme":"dark","accent":"default","backgroundUrl":"","backgroundDim":20,` +
		`"palette":{"primary":"#112233","secondary":"#AABBCC","accent":"#ff8800","background":"#000000"}}`
	if w := putSettings(router, withPalette); w.Code != http.StatusOK {
		t.Fatalf("PUT palette = %d, %s", w.Code, w.Body.String())
	}
	want := Settings{Theme: "dark", Accent: "default", BackgroundDim: 20,
		Palette: &Palette{Primary: "#112233", Secondary: "#AABBCC", Accent: "#ff8800", Background: "#000000"}}
	got, err := (&Store{db: store.db}).Get(ctx)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("persisted = %+v, %v", got, err)
	}
	var decoded Settings
	if err := json.Unmarshal(get(router, "/").Body.Bytes(), &decoded); err != nil || !reflect.DeepEqual(decoded, want) {
		t.Fatalf("GET = %+v, %v", decoded, err)
	}

	// Old clients that omit palette, or explicit null, clear it.
	for _, body := range []string{
		`{"theme":"dark","accent":"teal","backgroundUrl":"","backgroundDim":20}`,
		`{"theme":"dark","accent":"teal","backgroundUrl":"","backgroundDim":20,"palette":null}`,
	} {
		_ = putSettings(router, withPalette)
		if w := putSettings(router, body); w.Code != http.StatusOK || strings.Contains(w.Body.String(), "palette") {
			t.Fatalf("PUT %s = %d, %s", body, w.Code, w.Body.String())
		}
		if got, err := store.Get(ctx); err != nil || got.Palette != nil {
			t.Fatalf("palette not cleared: %+v, %v", got, err)
		}
	}

	// Invalid palettes are rejected without writing.
	before, _ := store.Get(ctx)
	base := `{"theme":"dark","accent":"teal","backgroundUrl":"","backgroundDim":20,"palette":%s}`
	for _, p := range []string{
		`{}`,
		`[]`,
		`{"primary":"#112233","secondary":"#112233","accent":"#112233"}`,
		`{"primary":"#112233","secondary":"#112233","accent":"#112233","background":null}`,
		`{"primary":"#112233","secondary":"#112233","accent":"#112233","background":"#123"}`,
		`{"primary":"112233","secondary":"#112233","accent":"#112233","background":"#112233"}`,
		`{"primary":"#11223G","secondary":"#112233","accent":"#112233","background":"#112233"}`,
		`{"primary":"#11223344","secondary":"#112233","accent":"#112233","background":"#112233"}`,
		`{"primary":"red","secondary":"#112233","accent":"#112233","background":"#112233"}`,
		`{"primary":" #112233","secondary":"#112233","accent":"#112233","background":"#112233"}`,
		`{"primary":"#112233","secondary":"#112233","accent":"#112233","background":"#112233","text":"#ffffff"}`,
	} {
		if w := putSettings(router, strings.Replace(base, "%s", p, 1)); w.Code != http.StatusBadRequest {
			t.Fatalf("palette %s = %d, %s", p, w.Code, w.Body.String())
		}
	}
	if after, err := store.Get(ctx); err != nil || !reflect.DeepEqual(after, before) {
		t.Fatalf("invalid palette changed settings: %+v, %v", after, err)
	}
}

func TestInternalBackgroundURLValidation(t *testing.T) {
	hash := strings.Repeat("ab", 32)
	for url, valid := range map[string]bool{
		BackgroundPath:                                 true,
		BackgroundPath + "?v=" + hash:                  true,
		BackgroundPath + "?v=" + strings.ToUpper(hash): false,
		BackgroundPath + "?v=abc":                      false,
		BackgroundPath + "?v=" + hash + "&x=1":         false,
		BackgroundPath + "?x=1":                        false,
		BackgroundPath + "/":                           false,
		BackgroundPath + "#x":                          false,
		"/api/v1/appearance":                           false,
		"/api/v1/appearance/background/../../secrets":  false,
		"api/v1/appearance/background":                 false,
		"/other.png":                                   false,
	} {
		s := Defaults()
		s.BackgroundURL = url
		if err := s.Validate(); (err == nil) != valid {
			t.Errorf("Validate(%q) = %v; want valid=%v", url, err, valid)
		}
	}
}
