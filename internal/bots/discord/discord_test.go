package discord

import (
	"testing"

	"github.com/bwmarrin/discordgo"

	"github.com/ebenderooock/loom/internal/bots"
)

func TestMarkersToDiscord(t *testing.T) {
	if got := markersToDiscord("hello *world*"); got != "hello **world**" {
		t.Fatalf("got %q", got)
	}
}

func TestStyleFor(t *testing.T) {
	cases := map[string]discordgo.ButtonStyle{
		"apr|r1":       discordgo.SuccessButton,
		"rej|r1":       discordgo.DangerButton,
		"req|movie|11": discordgo.PrimaryButton,
	}
	for data, want := range cases {
		if got := styleFor(data); got != want {
			t.Fatalf("styleFor(%q)=%v want %v", data, got, want)
		}
	}
}

func TestComponentsChunksRows(t *testing.T) {
	var buttons []bots.Button
	for i := 0; i < 6; i++ {
		buttons = append(buttons, bots.Button{Label: "x", Data: "req|movie|1"})
	}
	rows := components(buttons)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows for 6 buttons, got %d", len(rows))
	}
	first, ok := rows[0].(discordgo.ActionsRow)
	if !ok || len(first.Components) != 5 {
		t.Fatalf("expected first row of 5, got %+v", rows[0])
	}
	second := rows[1].(discordgo.ActionsRow)
	if len(second.Components) != 1 {
		t.Fatalf("expected second row of 1, got %d", len(second.Components))
	}
}

func TestComponentsEmpty(t *testing.T) {
	if components(nil) != nil {
		t.Fatal("expected nil components for no buttons")
	}
}
