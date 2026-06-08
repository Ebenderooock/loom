// Package mediafiles provides a single source of truth for recognized
// media file extensions used by the scanner, validator, and importer.
package mediafiles

import "strings"

// VideoExtensions lists all recognized video file extensions.
var VideoExtensions = map[string]bool{
	".mkv":  true,
	".mp4":  true,
	".avi":  true,
	".wmv":  true,
	".flv":  true,
	".mov":  true,
	".m4v":  true,
	".ts":   true,
	".m2ts": true,
	".vob":  true,
	".webm": true,
	".mpg":  true,
	".mpeg": true,
	".ogm":  true,
}

// AudioExtensions lists all recognized audio (music) file extensions.
var AudioExtensions = map[string]bool{
	".flac": true,
	".mp3":  true,
	".m4a":  true,
	".m4b":  true,
	".alac": true,
	".aac":  true,
	".ogg":  true,
	".oga":  true,
	".opus": true,
	".wav":  true,
	".wv":   true,
	".ape":  true,
	".aiff": true,
	".aif":  true,
	".wma":  true,
}

// DangerousExtensions are files that should never be imported.
var DangerousExtensions = map[string]bool{
	".exe": true,
	".bat": true,
	".cmd": true,
	".msi": true,
	".scr": true,
	".com": true,
	".vbs": true,
	".js":  true,
	".ps1": true,
	".sh":  true,
}

// ArchiveExtensions that might contain password-protected content.
var ArchiveExtensions = map[string]bool{
	".rar": true,
	".zip": true,
	".7z":  true,
	".tar": true,
	".gz":  true,
}

// IsVideo returns true if the file extension (with leading dot) is a
// recognized video format. The comparison is case-insensitive.
func IsVideo(ext string) bool {
	return VideoExtensions[strings.ToLower(ext)]
}

// IsAudio returns true if the file extension (with leading dot) is a
// recognized audio format. The comparison is case-insensitive.
func IsAudio(ext string) bool {
	return AudioExtensions[strings.ToLower(ext)]
}

// IsDangerous returns true if the extension should never be imported.
func IsDangerous(ext string) bool {
	return DangerousExtensions[strings.ToLower(ext)]
}

// IsArchive returns true if the extension is an archive format.
func IsArchive(ext string) bool {
	return ArchiveExtensions[strings.ToLower(ext)]
}
