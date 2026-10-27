package deluge

import (
	"encoding/json"
	"testing"
)

func TestQueuePosition(t *testing.T) {
	for _, tc := range []struct {
		payload string
		want    int
	}{
		{`{"queue":0}`, 0},
		{`{"queue":3}`, 3},
		{`{"queue":-1}`, -1},
		{`{}`, -1},
	} {
		t.Run(tc.payload, func(t *testing.T) {
			var raw torrentStatus
			if err := json.Unmarshal([]byte(tc.payload), &raw); err != nil {
				t.Fatal(err)
			}
			got := mapItem("hash", raw).QueuePosition
			if tc.want < 0 {
				if got != nil {
					t.Fatalf("want no queue position, got %d", *got)
				}
			} else if got == nil || *got != tc.want {
				t.Fatalf("queue position = %v, want %d", got, tc.want)
			}
		})
	}
}
