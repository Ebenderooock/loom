package torrentutil

import (
	"net/url"
	"strings"
)

var publicTrackers = []string{
	"udp://tracker.opentrackr.org:1337/announce",
	"udp://open.stealth.si:80/announce",
	"udp://exodus.desync.com:6969/announce",
	"udp://tracker.torrent.eu.org:451/announce",
	"udp://tracker.bittor.pw:1337/announce",
	"udp://public.popcorn-tracker.org:6969/announce",
	"udp://tracker.dler.org:6969/announce",
	"udp://open.demonii.com:1337/announce",
	"udp://glotorrents.pw:6969/announce",
	"udp://tracker.coppersurfer.tk:6969",
	"udp://torrent.gresille.org:80/announce",
	"udp://p4p.arenabg.com:1337",
	"udp://tracker.internetwarriors.net:1337",
}

// PublicTrackers returns the shared public tracker bootstrap list used for
// synthesized infohash-only magnets.
func PublicTrackers() []string {
	return append([]string(nil), publicTrackers...)
}

// BuildPublicMagnet constructs a magnet URI for infohash-only releases using a
// tracker-rich shape similar to TPB detail pages.
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
	for _, tr := range publicTrackers {
		v.Add("tr", tr)
	}
	return "magnet:?" + v.Encode()
}
