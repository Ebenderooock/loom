package music

import "testing"

func TestBindMusicQueryPostgresPlaceholders(t *testing.T) {
	query := "SELECT * FROM tracks WHERE album_id = ? AND title = '?' AND recording_mbid = ?"
	want := "SELECT * FROM tracks WHERE album_id = $1 AND title = '?' AND recording_mbid = $2"
	if got := bindMusicQuery(query, true); got != want {
		t.Fatalf("bindMusicQuery() = %q, want %q", got, want)
	}
	if got := bindMusicQuery(query, false); got != query {
		t.Fatalf("SQLite query changed: got %q, want %q", got, query)
	}
}
