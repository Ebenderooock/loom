package autosearch

import (
	"context"
	"log/slog"
	"testing"

	"github.com/ebenderooock/loom/internal/indexers"
)

type fakeBlocklist struct{ title, hash string }

func (f fakeBlocklist) IsReleaseBlocked(_ context.Context, title, hash string) (bool, error) {
	return (title != "" && title == f.title) || (hash != "" && hash == f.hash), nil
}

func TestEngineRejectsBlocklistedReleases(t *testing.T) {
	const hash = "0123456789abcdef0123456789abcdef01234567"
	e := &Engine{logger: slog.Default()}
	WithBlocklist(fakeBlocklist{title: "Bad.Release", hash: hash})(e)
	ctx := context.Background()

	cases := []struct {
		name string
		res  indexers.Result
		want bool
	}{
		{"title", indexers.Result{Title: "Bad.Release"}, true},
		{"infohash", indexers.Result{Title: "Renamed", Infohash: hash}, true},
		{"magnet", indexers.Result{Title: "Renamed", MagnetURI: "magnet:?xt=urn:btih:" + "0123456789ABCDEF0123456789ABCDEF01234567" + "&dn=x"}, true},
		{"clean", indexers.Result{Title: "Good.Release"}, false},
	}
	for _, tc := range cases {
		if got := e.isBlocklisted(ctx, tc.res); got != tc.want {
			t.Errorf("%s: isBlocklisted = %v, want %v", tc.name, got, tc.want)
		}
	}
}
