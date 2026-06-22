package torrent

import (
	"net/url"
	"strings"
)

// defaultTrackers are appended to infohash-only magnets.
// TCP trackers are first to stay functional in UDP-constrained clusters.
var defaultTrackers = []string{
	"https://tracker.opentrackr.org:443/announce",
	"http://tracker.opentrackr.org:1337/announce",
	"https://tracker.bt4g.com:443/announce",
	"https://tracker.tamersunion.org:443/announce",
	"https://tracker1.520.jp:443/announce",
	"https://opentracker.i2p.rocks:443/announce",
	"https://tracker.gbitt.info:443/announce",
	"http://tracker.gbitt.info:80/announce",
	"http://open.acgnxtracker.com:80/announce",
	"http://bt.okmp3.ru:2710/announce",
	"udp://tracker.opentrackr.org:1337/announce",
	"udp://open.stealth.si:80/announce",
	"udp://exodus.desync.com:6969/announce",
	"udp://tracker.torrent.eu.org:451/announce",
	"udp://open.demonii.com:1337/announce",
	"udp://tracker.dler.org:6969/announce",
	"udp://p4p.arenabg.com:1337/announce",
}

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
