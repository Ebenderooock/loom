package importlists

import (
	"reflect"
	"testing"
)

func TestEncodeDecodeGenres(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		enc  string
		out  []string
	}{
		{"nil", nil, "", nil},
		{"empty", []string{}, "", nil},
		{"single", []string{"Action"}, "Action", []string{"Action"}},
		{"multi", []string{"Action", "Sci-Fi & Fantasy"}, "Action|Sci-Fi & Fantasy", []string{"Action", "Sci-Fi & Fantasy"}},
		{"trims blanks", []string{" Drama ", "", "  "}, "Drama", []string{"Drama"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := encodeGenres(c.in); got != c.enc {
				t.Fatalf("encode: want %q got %q", c.enc, got)
			}
			if got := decodeGenres(c.enc); !reflect.DeepEqual(got, c.out) {
				t.Fatalf("decode: want %v got %v", c.out, got)
			}
		})
	}
}

func TestDecodeGenresTolerant(t *testing.T) {
	got := decodeGenres("Action| |Comedy|")
	want := []string{"Action", "Comedy"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v got %v", want, got)
	}
}
