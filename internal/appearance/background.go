package appearance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // register JPEG decoder
	_ "image/png"  // register PNG decoder
	"net/http"
	"time"

	_ "golang.org/x/image/webp" // register WebP decoder
)

const (
	// MaxBackgroundBytes bounds an uploaded background image file.
	MaxBackgroundBytes = 5 << 20
	// MaxBackgroundSide bounds either image dimension.
	MaxBackgroundSide = 8192
	// MaxBackgroundPixels bounds total pixels (8K UHD) to limit decode memory.
	MaxBackgroundPixels = 7680 * 4320
)

// ErrBackgroundNotFound means no uploaded image matches the request.
var ErrBackgroundNotFound = errors.New("background image not found")

// ImageError is a client-facing upload validation failure.
type ImageError struct {
	Status  int
	Message string
}

func (e *ImageError) Error() string { return e.Message }

// BackgroundImage is a validated raster image persisted in the database.
type BackgroundImage struct {
	ContentType string    `json:"contentType"`
	SHA256      string    `json:"sha256"`
	Size        int       `json:"size"`
	Width       int       `json:"width"`
	Height      int       `json:"height"`
	UploadedAt  time.Time `json:"uploadedAt"`
	Data        []byte    `json:"data"` // base64 in JSON
}

// URL returns the content-addressed internal URL for the image.
func (img *BackgroundImage) URL() string { return BackgroundPath + "?v=" + img.SHA256 }

var allowedFormats = map[string]string{
	"image/png":  "png",
	"image/jpeg": "jpeg",
	"image/webp": "webp",
}

// ValidateBackgroundImage sniffs and fully decodes data, accepting only
// bounded PNG, JPEG, or WebP rasters. The client-declared type is ignored.
func ValidateBackgroundImage(data []byte) (*BackgroundImage, error) {
	if len(data) == 0 {
		return nil, &ImageError{http.StatusBadRequest, "image file is empty"}
	}
	if len(data) > MaxBackgroundBytes {
		return nil, &ImageError{http.StatusRequestEntityTooLarge, "image must not exceed 5 MiB"}
	}
	contentType := http.DetectContentType(data)
	wantFormat, ok := allowedFormats[contentType]
	if !ok {
		return nil, &ImageError{http.StatusUnsupportedMediaType, "image must be a PNG, JPEG, or WebP file"}
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || format != wantFormat {
		return nil, &ImageError{http.StatusBadRequest, "image could not be decoded"}
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > MaxBackgroundSide || cfg.Height > MaxBackgroundSide ||
		cfg.Width*cfg.Height > MaxBackgroundPixels {
		return nil, &ImageError{http.StatusBadRequest, fmt.Sprintf(
			"image dimensions must be at most %d pixels per side and %d pixels total", MaxBackgroundSide, MaxBackgroundPixels)}
	}
	// Full decode (dimensions already bounded) rejects truncated or corrupt files.
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return nil, &ImageError{http.StatusBadRequest, "image could not be decoded"}
	}
	sum := sha256.Sum256(data)
	return &BackgroundImage{
		ContentType: contentType,
		SHA256:      hex.EncodeToString(sum[:]),
		Size:        len(data),
		Width:       cfg.Width,
		Height:      cfg.Height,
		UploadedAt:  time.Now().UTC(),
		Data:        data,
	}, nil
}
