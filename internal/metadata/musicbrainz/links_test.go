package musicbrainz

import (
	"encoding/json"
	"testing"
)

func TestMapURLRelations(t *testing.T) {
	raw := `[
		{"type":"free streaming","url":{"resource":"https://open.spotify.com/artist/6XyY86QOPPrYVGvF9ch6wz"}},
		{"type":"purchase for download","url":{"resource":"https://itunes.apple.com/us/artist/id148662"}},
		{"type":"streaming","url":{"resource":"https://music.apple.com/us/artist/148662"}},
		{"type":"youtube music","url":{"resource":"https://music.youtube.com/channel/UCx"}},
		{"type":"youtube","url":{"resource":"https://www.youtube.com/channel/UCZ"}},
		{"type":"official homepage","url":{"resource":"https://www.linkinpark.com/"}},
		{"type":"image","url":{"resource":"https://commons.wikimedia.org/wiki/File:LinkinParkBerlin2010.jpg"}},
		{"type":"wikidata","url":{"resource":"https://www.wikidata.org/wiki/Q261"}},
		{"type":"other databases","url":{"resource":"https://rateyourmusic.com/artist/linkin_park"}},
		{"type":"free streaming","url":{"resource":"javascript:alert(1)"}},
		{"type":"free streaming","url":{"resource":"https://www.deezer.com/artist/92"}},
		{"type":"free streaming","url":{"resource":"https://www.deezer.com/artist/14788305"}}
	]`
	var rels []RelationResponse
	if err := json.Unmarshal([]byte(raw), &rels); err != nil {
		t.Fatal(err)
	}
	links, image := mapURLRelations(rels)
	if image != "https://commons.wikimedia.org/wiki/Special:FilePath/LinkinParkBerlin2010.jpg?width=1000" {
		t.Fatalf("image = %q", image)
	}
	want := []struct{ typ, url string }{
		{LinkWebsite, "https://www.linkinpark.com/"},
		{LinkSpotify, "https://open.spotify.com/artist/6XyY86QOPPrYVGvF9ch6wz"},
		{LinkAppleMusic, "https://music.apple.com/us/artist/148662"},
		{LinkYouTubeMusic, "https://music.youtube.com/channel/UCx"},
		{LinkDeezer, "https://www.deezer.com/artist/92"},
		{LinkYouTube, "https://www.youtube.com/channel/UCZ"},
		{LinkWikidata, "https://www.wikidata.org/wiki/Q261"},
	}
	if len(links) != len(want) {
		t.Fatalf("links = %+v", links)
	}
	for i, w := range want {
		if links[i].Type != w.typ || links[i].URL != w.url {
			t.Errorf("link %d = %+v, want %s %s", i, links[i], w.typ, w.url)
		}
	}
}

func TestMapArtistLifeSpanAndRating(t *testing.T) {
	raw := `{"id":"x","name":"Linkin Park","begin-area":{"name":"Agoura Hills"},
		"life-span":{"begin":"1996","end":null,"ended":false},
		"rating":{"value":4.4,"votes-count":42}}`
	var resp ArtistResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatal(err)
	}
	a := MapArtist(&resp)
	if a.BeginArea != "Agoura Hills" || a.BeginDate != "1996" || a.Ended || a.Rating != 4.4 || a.RatingVotes != 42 {
		t.Fatalf("mapped = %+v", a)
	}
	var noRating ArtistResponse
	_ = json.Unmarshal([]byte(`{"id":"y","rating":{"value":null,"votes-count":0}}`), &noRating)
	if got := MapArtist(&noRating); got.Rating != 0 {
		t.Fatalf("rating = %v", got.Rating)
	}
}
