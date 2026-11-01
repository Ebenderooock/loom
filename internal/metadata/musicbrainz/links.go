package musicbrainz

import (
	"net/url"
	"strings"

	"github.com/ebenderooock/loom/internal/metadata"
)

// Link types surfaced to clients, in display order.
const (
	LinkWebsite      = "website"
	LinkSpotify      = "spotify"
	LinkAppleMusic   = "apple_music"
	LinkYouTubeMusic = "youtube_music"
	LinkTidal        = "tidal"
	LinkDeezer       = "deezer"
	LinkAmazonMusic  = "amazon_music"
	LinkBandcamp     = "bandcamp"
	LinkSoundCloud   = "soundcloud"
	LinkYouTube      = "youtube"
	LinkWikipedia    = "wikipedia"
	LinkWikidata     = "wikidata"
	LinkDiscogs      = "discogs"
	LinkAllMusic     = "allmusic"
	LinkLastFM       = "lastfm"
	LinkInstagram    = "instagram"
	LinkFacebook     = "facebook"
	LinkTwitter      = "twitter"
	LinkTikTok       = "tiktok"
)

var linkOrder = []string{
	LinkWebsite, LinkSpotify, LinkAppleMusic, LinkYouTubeMusic, LinkTidal, LinkDeezer,
	LinkAmazonMusic, LinkBandcamp, LinkSoundCloud, LinkYouTube, LinkWikipedia, LinkWikidata,
	LinkDiscogs, LinkAllMusic, LinkLastFM, LinkInstagram, LinkFacebook, LinkTwitter, LinkTikTok,
}

// commonsImageWidth is the width requested for Wikimedia Commons thumbnails.
const commonsImageWidth = "1000"

// mapURLRelations picks one link per known service from MusicBrainz url
// relationships and returns the Wikimedia Commons image, if any, as a
// directly loadable thumbnail URL.
func mapURLRelations(rels []RelationResponse) (links []metadata.ExternalLink, imageURL string) {
	found := make(map[string]string)
	for _, rel := range rels {
		if rel.URL == nil || rel.URL.Resource == "" {
			continue
		}
		u, err := url.Parse(rel.URL.Resource)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			continue
		}
		if rel.Type == "image" {
			if imageURL == "" {
				imageURL = commonsImageURL(u)
			}
			continue
		}
		kind := classifyLink(rel.Type, u)
		if kind == "" {
			continue
		}
		// Apple's legacy iTunes Store links are kept only as a fallback.
		if prev, ok := found[kind]; ok && (kind != LinkAppleMusic || !strings.Contains(prev, "itunes.apple.com")) {
			continue
		}
		found[kind] = rel.URL.Resource
	}
	for _, kind := range linkOrder {
		if v, ok := found[kind]; ok {
			links = append(links, metadata.ExternalLink{Type: kind, URL: v})
		}
	}
	return links, imageURL
}

func classifyLink(relType string, u *url.URL) string {
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	hostIs := func(domain string) bool { return host == domain || strings.HasSuffix(host, "."+domain) }
	switch {
	case relType == "official homepage":
		return LinkWebsite
	case hostIs("spotify.com"):
		return LinkSpotify
	case host == "music.apple.com" || host == "itunes.apple.com":
		return LinkAppleMusic
	case host == "music.youtube.com":
		return LinkYouTubeMusic
	case hostIs("youtube.com") || host == "youtu.be":
		return LinkYouTube
	case hostIs("tidal.com"):
		return LinkTidal
	case hostIs("deezer.com"):
		return LinkDeezer
	case strings.HasPrefix(host, "music.amazon."):
		return LinkAmazonMusic
	case hostIs("bandcamp.com"):
		return LinkBandcamp
	case hostIs("soundcloud.com"):
		return LinkSoundCloud
	case hostIs("wikipedia.org"):
		return LinkWikipedia
	case hostIs("wikidata.org"):
		return LinkWikidata
	case hostIs("discogs.com"):
		return LinkDiscogs
	case hostIs("allmusic.com"):
		return LinkAllMusic
	case hostIs("last.fm"):
		return LinkLastFM
	case hostIs("instagram.com"):
		return LinkInstagram
	case hostIs("facebook.com"):
		return LinkFacebook
	case hostIs("twitter.com") || host == "x.com":
		return LinkTwitter
	case hostIs("tiktok.com"):
		return LinkTikTok
	}
	return ""
}

// commonsImageURL converts a Commons file page into a thumbnail URL.
func commonsImageURL(u *url.URL) string {
	host := strings.ToLower(u.Hostname())
	if host != "commons.wikimedia.org" {
		return ""
	}
	file, ok := strings.CutPrefix(u.EscapedPath(), "/wiki/File:")
	if !ok || file == "" {
		return ""
	}
	return "https://commons.wikimedia.org/wiki/Special:FilePath/" + file + "?width=" + commonsImageWidth
}

func mapRating(r *RatingResponse) (float64, int) {
	if r == nil || r.Value == nil {
		return 0, 0
	}
	return *r.Value, r.VotesCount
}
