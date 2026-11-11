package music

import (
	"regexp"
	"strings"
)

var (
	albumEditionSuffix  = regexp.MustCompile(`(?i)\s+(?:[-–—]\s*)?(?:super\s+deluxe|deluxe|expanded|special|collector'?s)(?:\s+edition|\s+version)?$`)
	albumEditionLabel   = regexp.MustCompile(`(?i)^(?:super\s+deluxe|deluxe|expanded|special|collector'?s)(?:\s+edition|\s+version)?$`)
	albumQualityLabel   = regexp.MustCompile(`(?i)^(?:(?:16|24|32)\s*[- ]?bit(?:\s*[/, -]\s*\d+(?:\.\d+)?\s*(?:khz)?)?|hi[- ]?res(?:olution)?|flac|alac)$`)
	albumBitDepthSuffix = regexp.MustCompile(`(?i)\s+(?:[-–—]\s*)?(?:16|24|32)\s*[- ]?bit(?:\s*[/, -]\s*\d+(?:\.\d+)?\s*(?:khz)?)?$`)
)

// BaseAlbumTitle removes only known trailing edition/quality labels. Unlike
// release-name parsing, this preserves meaningful qualifiers such as "(Live)"
// and "(Remixes)". Callers must still verify artist and track identity and
// reject multiple matching albums.
func BaseAlbumTitle(title string) string {
	base := strings.TrimSpace(title)
	for {
		next := base
		if loc := trailingQualifierRe.FindStringIndex(base); loc != nil {
			block := strings.TrimSpace(base[loc[0]:loc[1]])
			label := strings.TrimSpace(block[1 : len(block)-1])
			if albumEditionLabel.MatchString(label) || albumQualityLabel.MatchString(label) {
				next = strings.TrimSpace(base[:loc[0]])
			}
		}
		next = strings.TrimSpace(albumBitDepthSuffix.ReplaceAllString(next, ""))
		next = strings.TrimSpace(albumEditionSuffix.ReplaceAllString(next, ""))
		if next == "" || next == base {
			return base
		}
		base = next
	}
}
