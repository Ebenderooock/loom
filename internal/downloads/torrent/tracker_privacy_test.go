package torrent

import "testing"

func TestMagnetLikelyPrivate(t *testing.T) {
	tests := []struct {
		name    string
		magnet  string
		private bool
	}{
		{
			name:    "no trackers",
			magnet:  "magnet:?xt=urn:btih:3369311b1306d629d8e1c02abe4f5235cd2f44e1",
			private: false,
		},
		{
			name:    "public open trackers (EZTV/YTS style)",
			magnet:  "magnet:?xt=urn:btih:3369311b1306d629d8e1c02abe4f5235cd2f44e1&tr=udp%3A%2F%2Ftracker.opentrackr.org%3A1337%2Fannounce&tr=udp%3A%2F%2Fopen.demonii.com%3A1337%2Fannounce",
			private: false,
		},
		{
			name:    "passkey in query param",
			magnet:  "magnet:?xt=urn:btih:3369311b1306d629d8e1c02abe4f5235cd2f44e1&tr=https%3A%2F%2Ftracker.example.org%2Fannounce%3Fpasskey%3Ddeadbeefdeadbeefdeadbeef",
			private: true,
		},
		{
			name:    "torrent_pass + authkey (Gazelle style)",
			magnet:  "magnet:?xt=urn:btih:3369311b1306d629d8e1c02abe4f5235cd2f44e1&tr=https%3A%2F%2Ftracker.example.org%2Fannounce%3Fauthkey%3Dabc%26torrent_pass%3Dxyz",
			private: true,
		},
		{
			name:    "passkey embedded in path",
			magnet:  "magnet:?xt=urn:btih:3369311b1306d629d8e1c02abe4f5235cd2f44e1&tr=https%3A%2F%2Ftracker.example.org%2F0123456789abcdef0123456789abcdef%2Fannounce",
			private: true,
		},
		{
			name:    "mixed public and private => private",
			magnet:  "magnet:?xt=urn:btih:3369311b1306d629d8e1c02abe4f5235cd2f44e1&tr=udp%3A%2F%2Ftracker.opentrackr.org%3A1337%2Fannounce&tr=https%3A%2F%2Ftracker.example.org%2Fannounce%3Fpasskey%3Ddeadbeefdeadbeefdeadbeef",
			private: true,
		},
		{
			name:    "unparseable magnet => private (safe)",
			magnet:  "magnet:?xt=urn:btih:3369311b1306d629d8e1c02abe4f5235cd2f44e1&tr=ht\x7ftp://x",
			private: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := magnetLikelyPrivate(tt.magnet); got != tt.private {
				t.Errorf("magnetLikelyPrivate(%q) = %v, want %v", tt.magnet, got, tt.private)
			}
		})
	}
}

func TestLooksLikePasskey(t *testing.T) {
	tests := []struct {
		seg  string
		want bool
	}{
		{"announce", false},
		{"", false},
		{"a", false},
		{"0123456789abcdef0123", true}, // 20 hex chars
		{"0123456789abcdef0123456789abcdef", true}, // 32 hex chars
		{"AbCdEfGhIjKlMnOpQrStUv", true},           // 22 alphanumeric
		{"this-has-a-dash-and-is-long", false},     // non-alphanumeric
		{"announce.php", false},                    // short with dot
	}
	for _, tt := range tests {
		if got := looksLikePasskey(tt.seg); got != tt.want {
			t.Errorf("looksLikePasskey(%q) = %v, want %v", tt.seg, got, tt.want)
		}
	}
}

func TestTrackerHasCredential(t *testing.T) {
	tests := []struct {
		tracker string
		want    bool
	}{
		{"udp://tracker.opentrackr.org:1337/announce", false},
		{"http://tracker.openbittorrent.com:80/announce", false},
		{"https://tracker.example.org/announce?passkey=secretsecretsecret", true},
		{"https://tracker.example.org/0123456789abcdef0123456789abcdef/announce", true},
		{"https://tracker.example.org/announce?apikey=abc123", true},
	}
	for _, tt := range tests {
		if got := trackerHasCredential(tt.tracker); got != tt.want {
			t.Errorf("trackerHasCredential(%q) = %v, want %v", tt.tracker, got, tt.want)
		}
	}
}
