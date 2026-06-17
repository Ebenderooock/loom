package torrent

import (
	"net/url"
	"strings"
)

// BuildPublicMagnet constructs a magnet URI for infohash-only releases
// using the same tracker-rich shape TPB exposes from its detail page.
// This is notably more reliable than a bare xt=urn:btih magnet in
// containerized environments where DHT discovery is spotty.
func BuildPublicMagnet(infohash, title string) string {
	infohash = strings.TrimSpace(infohash)
	if infohash == "" {
		return ""
	}

	v := url.Values{}
	v.Set("xt", "urn:btih:"+infohash)
	if title = strings.TrimSpace(title); title != "" {
		v.Set("dn", title)
	}
	for _, tr := range defaultTrackers {
		v.Add("tr", tr)
	}
	return "magnet:?" + v.Encode()
}
