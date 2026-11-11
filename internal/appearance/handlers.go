package appearance

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

const maxBodyBytes = 16 * 1024

// Router must be mounted inside the authenticated router. adminMW protects
// writes; nil follows the server's auth-disabled convention.
func Router(store *Store, logger *slog.Logger, adminMW func(http.Handler) http.Handler) chi.Router {
	if logger == nil {
		logger = slog.Default()
	}
	if adminMW == nil {
		adminMW = func(next http.Handler) http.Handler { return next }
	}
	r := chi.NewRouter()
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		settings, err := store.Get(r.Context())
		if err != nil {
			logger.Error("appearance: failed to read settings", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read appearance settings"})
			return
		}
		writeJSON(w, http.StatusOK, settings)
	})
	r.With(adminMW).Put("/", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		// Pointers distinguish omitted/null fields from valid zero values. PUT
		// replaces the full object, never merges a partial update.
		var body struct {
			Theme         *string `json:"theme"`
			Accent        *string `json:"accent"`
			BackgroundURL *string `json:"backgroundUrl"`
			BackgroundDim *int    `json:"backgroundDim"`
			// Optional: omitted or null clears the custom palette.
			Palette *struct {
				Primary    *string `json:"primary"`
				Secondary  *string `json:"secondary"`
				Accent     *string `json:"accent"`
				Background *string `json:"background"`
			} `json:"palette"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		err := decoder.Decode(&body)
		if err == nil {
			var extra any
			err = decoder.Decode(&extra)
			if errors.Is(err, io.EOF) {
				err = nil
			} else if err == nil {
				err = errors.New("multiple JSON values")
			}
		}
		if err != nil {
			status := http.StatusBadRequest
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				status = http.StatusRequestEntityTooLarge
			}
			writeJSON(w, status, map[string]string{"error": "invalid appearance body"})
			return
		}
		if body.Theme == nil || body.Accent == nil || body.BackgroundURL == nil || body.BackgroundDim == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "all appearance fields are required"})
			return
		}
		settings := Settings{Theme: *body.Theme, Accent: *body.Accent, BackgroundURL: *body.BackgroundURL, BackgroundDim: *body.BackgroundDim}
		if p := body.Palette; p != nil {
			if p.Primary == nil || p.Secondary == nil || p.Accent == nil || p.Background == nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "palette requires primary, secondary, accent, and background"})
				return
			}
			settings.Palette = &Palette{Primary: *p.Primary, Secondary: *p.Secondary, Accent: *p.Accent, Background: *p.Background}
		}
		if err := settings.Validate(); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := store.Set(r.Context(), settings); err != nil {
			if errors.Is(err, ErrBackgroundUnavailable) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			logger.Error("appearance: failed to write settings", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save appearance settings"})
			return
		}
		writeJSON(w, http.StatusOK, settings)
	})
	r.Get("/background", func(w http.ResponseWriter, r *http.Request) {
		hash := r.URL.Query().Get("v")
		if hash != "" {
			if _, ok := InternalBackground(BackgroundPath + "?v=" + hash); !ok {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "v must be a lowercase hex SHA-256 digest"})
				return
			}
		}
		img, err := store.Background(r.Context(), hash)
		if errors.Is(err, ErrBackgroundNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "background image not found"})
			return
		}
		if err != nil {
			logger.Error("appearance: failed to read background image", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read background image"})
			return
		}
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
		h.Set("ETag", `"`+img.SHA256+`"`)
		if hash != "" {
			// Content-addressed URLs never change, so authenticated clients may cache them.
			h.Set("Cache-Control", "private, max-age=31536000, immutable")
		} else {
			h.Set("Cache-Control", "private, no-cache")
		}
		if etagMatches(r.Header.Get("If-None-Match"), img.SHA256) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		h.Set("Content-Type", img.ContentType)
		h.Set("Content-Length", strconv.Itoa(len(img.Data)))
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write(img.Data)
		}
	})
	r.With(adminMW).Post("/background", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxUploadBodyBytes)
		data, err := readUploadedImage(r)
		if err != nil {
			var imgErr *ImageError
			if errors.As(err, &imgErr) {
				writeJSON(w, imgErr.Status, map[string]string{"error": imgErr.Message})
				return
			}
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart upload"})
			return
		}
		img, err := ValidateBackgroundImage(data)
		if err != nil {
			var imgErr *ImageError
			if errors.As(err, &imgErr) {
				writeJSON(w, imgErr.Status, map[string]string{"error": imgErr.Message})
				return
			}
			logger.Error("appearance: failed to validate background image", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to process background image"})
			return
		}
		if err := store.StageBackground(r.Context(), img); err != nil {
			logger.Error("appearance: failed to store background image", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to store background image"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"url": img.URL()})
	})
	return r
}

// maxUploadBodyBytes allows the 5 MiB file plus multipart framing.
const maxUploadBodyBytes = MaxBackgroundBytes + 64*1024

var errTooLarge = &ImageError{http.StatusRequestEntityTooLarge, "image must not exceed 5 MiB"}

// readUploadedImage streams the multipart body (no temp files) and returns the
// single "image" part. Any other field is rejected.
func readUploadedImage(r *http.Request) ([]byte, error) {
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, &ImageError{http.StatusBadRequest, "request must be multipart/form-data with an image field"}
	}
	var data []byte
	found := false
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, uploadReadError(err)
		}
		if part.FormName() != "image" || found {
			_ = part.Close()
			return nil, &ImageError{http.StatusBadRequest, "upload must contain exactly one image field"}
		}
		found = true
		data, err = io.ReadAll(io.LimitReader(part, MaxBackgroundBytes+1))
		_ = part.Close()
		if err != nil {
			return nil, uploadReadError(err)
		}
		if len(data) > MaxBackgroundBytes {
			return nil, errTooLarge
		}
	}
	if !found {
		return nil, &ImageError{http.StatusBadRequest, "upload must contain exactly one image field"}
	}
	return data, nil
}

func uploadReadError(err error) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return errTooLarge
	}
	return &ImageError{http.StatusBadRequest, "invalid multipart upload"}
}

func etagMatches(header, hash string) bool {
	for _, tag := range strings.Split(header, ",") {
		tag = strings.TrimPrefix(strings.TrimSpace(tag), "W/")
		if tag == "*" || tag == `"`+hash+`"` {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
