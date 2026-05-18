package imports

import (
	"context"
	"path/filepath"
	"strings"
)

// DangerousFileSpec rejects potentially dangerous files.
type DangerousFileSpec struct{}

func (s *DangerousFileSpec) Name() string { return "NotDangerous" }

var dangerousExtensions = map[string]bool{
	".exe": true, ".bat": true, ".cmd": true, ".com": true,
	".msi": true, ".ps1": true, ".sh": true, ".vbs": true,
	".js": true, ".jar": true, ".scr": true, ".pif": true,
}

func (s *DangerousFileSpec) IsSatisfiedBy(_ context.Context, c *ImportCandidate) *ImportRejection {
	ext := strings.ToLower(filepath.Ext(c.SourcePath))
	if dangerousExtensions[ext] {
		return &ImportRejection{
			Reason:  RejectionDangerousFile,
			Message: "File has a potentially dangerous extension: " + ext,
		}
	}
	return nil
}
