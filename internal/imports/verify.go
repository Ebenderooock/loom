package imports

import (
	"fmt"
	"os"
)

// ImportVerifier checks that an imported file arrived correctly at its
// destination.
type ImportVerifier struct{}

// VerifyResult holds the outcome of a post-import verification.
type VerifyResult struct {
	OK     bool
	Reason string // if not OK, why
}

// Verify checks that the imported file exists, is non-zero, and matches
// the expected source size.
func (v *ImportVerifier) Verify(destPath string, expectedSize int64) VerifyResult {
	info, err := os.Stat(destPath)
	if err != nil {
		return VerifyResult{
			OK:     false,
			Reason: fmt.Sprintf("destination file not found: %v", err),
		}
	}

	if info.Size() == 0 {
		return VerifyResult{
			OK:     false,
			Reason: "destination file is zero bytes",
		}
	}

	if expectedSize > 0 && info.Size() != expectedSize {
		return VerifyResult{
			OK:     false,
			Reason: fmt.Sprintf("size mismatch: expected %d bytes, got %d bytes", expectedSize, info.Size()),
		}
	}

	return VerifyResult{OK: true}
}
