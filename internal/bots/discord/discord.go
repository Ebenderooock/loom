// Package discord implements a Discord transport for the Loom request bots using
// slash commands and message components over an outbound gateway connection
// (discordgo). All responses are sent ephemerally so request data never leaks
// into shared channels, and only the non-privileged Guilds intent is required.
package discord

import (
	"context"
	"log/slog"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/ebenderooock/loom/internal/bots"
)

// Handler turns an inbound command into a reply (satisfied by *bots.Service).
type Handler interface {
	Handle(ctx context.Context, cmd bots.Command) bots.Reply
}

// Bot is a Discord slash-command transport.
type Bot struct {
	token   string
	handler Handler
	logger  *slog.Logger

	mu      sync.Mutex
	lastErr string
}

// New constructs a Discord bot transport.
func New(token string, handler Handler, logger *slog.Logger) *Bot {
	if logger == nil {
		logger = slog.Default()
	}
	return &Bot{token: token, handler: handler, logger: logger}
}

// slashCommands are the application commands the bot registers.
var slashCommands = []*discordgo.ApplicationCommand{
	{Name: "search", Description: "Search for a movie or show to request", Options: []*discordgo.ApplicationCommandOption{
		{Type: discordgo.ApplicationCommandOptionString, Name: "title", Description: "Title to search for", Required: true},
	}},
	{Name: "status", Description: "Show your media requests"},
	{Name: "link", Description: "Link this Discord account to your Loom account"},
	{Name: "pending", Description: "Review pending requests (admins only)"},
	{Name: "help", Description: "How to use the Loom request bot"},
}

// Run opens the gateway, registers commands, and serves until ctx is cancelled.
func (b *Bot) Run(ctx context.Context) error {
	session, err := discordgo.New("Bot " + b.token)
	if err != nil {
		b.setErr(err.Error())
		return err
	}
	session.Identify.Intents = discordgo.IntentsGuilds

	session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		b.onInteraction(ctx, s, i)
	})
	// Register commands per-guild on join for instant availability.
	session.AddHandler(func(s *discordgo.Session, g *discordgo.GuildCreate) {
		b.registerGuildCommands(s, g.ID)
	})

	if err := session.Open(); err != nil {
		b.setErr(err.Error())
		return err
	}
	b.logger.Info("discord bot: connected", "user", safeUser(session))
	// Register global commands too (covers DMs; eventual consistency).
	b.registerGlobalCommands(session)
	b.setErr("")

	<-ctx.Done()
	b.logger.Info("discord bot: stopping")
	_ = session.Close()
	return ctx.Err()
}

// LastError returns the most recent transport error, for health reporting.
func (b *Bot) LastError() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastErr
}

func (b *Bot) setErr(s string) {
	b.mu.Lock()
	b.lastErr = s
	b.mu.Unlock()
}

func (b *Bot) registerGlobalCommands(s *discordgo.Session) {
	appID := s.State.User.ID
	for _, c := range slashCommands {
		if _, err := s.ApplicationCommandCreate(appID, "", c); err != nil {
			b.logger.Warn("discord bot: register global command", "cmd", c.Name, "err", err)
		}
	}
}

func (b *Bot) registerGuildCommands(s *discordgo.Session, guildID string) {
	appID := s.State.User.ID
	for _, c := range slashCommands {
		if _, err := s.ApplicationCommandCreate(appID, guildID, c); err != nil {
			b.logger.Warn("discord bot: register guild command", "guild", guildID, "cmd", c.Name, "err", err)
		}
	}
}

func (b *Bot) onInteraction(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) {
	cmd, ok := toCommand(i)
	if !ok {
		return
	}
	reply := b.handler.Handle(ctx, cmd)
	resp := &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content:    markersToDiscord(reply.Text),
			Flags:      discordgo.MessageFlagsEphemeral,
			Components: components(reply.Buttons),
		},
	}
	if err := s.InteractionRespond(i.Interaction, resp); err != nil {
		b.logger.Warn("discord bot: respond failed", "err", err)
	}
}

// toCommand converts a slash command or component interaction into a bots.Command.
func toCommand(i *discordgo.InteractionCreate) (bots.Command, bool) {
	id, handle := identity(i)
	cmd := bots.Command{
		Platform:         bots.PlatformDiscord,
		ExternalID:       id,
		ExternalUsername: handle,
	}
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		data := i.ApplicationCommandData()
		text := "/" + data.Name
		for _, opt := range data.Options {
			if opt.Type == discordgo.ApplicationCommandOptionString {
				text += " " + opt.StringValue()
			}
		}
		cmd.Text = text
		return cmd, true
	case discordgo.InteractionMessageComponent:
		cmd.Callback = i.MessageComponentData().CustomID
		return cmd, true
	default:
		return bots.Command{}, false
	}
}

// identity extracts the user id and handle from a guild or DM interaction.
func identity(i *discordgo.InteractionCreate) (id, handle string) {
	u := i.User
	if u == nil && i.Member != nil {
		u = i.Member.User
	}
	if u == nil {
		return "", ""
	}
	return u.ID, "@" + u.Username
}

// components renders bot buttons as Discord action rows (max 5 buttons/row).
func components(buttons []bots.Button) []discordgo.MessageComponent {
	if len(buttons) == 0 {
		return nil
	}
	var rows []discordgo.MessageComponent
	var row discordgo.ActionsRow
	for _, btn := range buttons {
		row.Components = append(row.Components, discordgo.Button{
			Label:    truncateLabel(btn.Label),
			Style:    styleFor(btn.Data),
			CustomID: btn.Data,
		})
		if len(row.Components) == 5 {
			rows = append(rows, row)
			row = discordgo.ActionsRow{}
		}
	}
	if len(row.Components) > 0 {
		rows = append(rows, row)
	}
	return rows
}

func styleFor(data string) discordgo.ButtonStyle {
	switch {
	case strings.HasPrefix(data, "apr|"):
		return discordgo.SuccessButton
	case strings.HasPrefix(data, "rej|"):
		return discordgo.DangerButton
	default:
		return discordgo.PrimaryButton
	}
}

// markersToDiscord converts the brain's `*emphasis*` to Discord bold `**`.
func markersToDiscord(s string) string { return strings.ReplaceAll(s, "*", "**") }

func truncateLabel(s string) string {
	const max = 80 // Discord button label limit
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

func safeUser(s *discordgo.Session) string {
	if s.State != nil && s.State.User != nil {
		return s.State.User.Username
	}
	return "?"
}
