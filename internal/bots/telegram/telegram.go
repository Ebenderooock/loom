// Package telegram implements a Telegram transport for the Loom request bots.
// It connects outbound via long-polling getUpdates (no inbound webhook needed,
// which suits NAT'd deployments) and renders the platform-agnostic bot replies
// as messages with inline keyboards.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ebenderooock/loom/internal/bots"
)

// Handler turns an inbound command into a reply (satisfied by *bots.Service).
type Handler interface {
	Handle(ctx context.Context, cmd bots.Command) bots.Reply
}

// Bot is a Telegram long-polling transport.
type Bot struct {
	token   string
	handler Handler
	client  *http.Client
	logger  *slog.Logger
	apiBase string // override for tests; defaults to https://api.telegram.org

	mu       sync.Mutex
	lastErr  string
	username string
}

// New constructs a Telegram bot transport.
func New(token string, handler Handler, logger *slog.Logger) *Bot {
	if logger == nil {
		logger = slog.Default()
	}
	return &Bot{
		token:   token,
		handler: handler,
		client:  &http.Client{Timeout: 65 * time.Second},
		logger:  logger,
		apiBase: "https://api.telegram.org",
	}
}

// Run polls for updates until ctx is cancelled. It is resilient to transient
// network/API errors, backing off between failures.
func (b *Bot) Run(ctx context.Context) error {
	b.logger.Info("telegram bot: starting")
	var offset int64
	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			b.logger.Info("telegram bot: stopped")
			return ctx.Err()
		default:
		}

		updates, err := b.getUpdates(ctx, offset, 30)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			b.setErr(err.Error())
			b.logger.Warn("telegram bot: getUpdates failed", "err", err)
			if !sleep(ctx, backoff) {
				return ctx.Err()
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
		b.setErr("")
		for _, u := range updates {
			if u.UpdateID >= offset {
				offset = u.UpdateID + 1
			}
			b.process(ctx, u)
		}
	}
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

func (b *Bot) process(ctx context.Context, u update) {
	switch {
	case u.CallbackQuery != nil:
		cq := u.CallbackQuery
		_ = b.answerCallback(ctx, cq.ID)
		if cq.Message == nil {
			return
		}
		cmd := bots.Command{
			Platform:         bots.PlatformTelegram,
			ExternalID:       strconv.FormatInt(cq.From.ID, 10),
			ExternalUsername: cq.From.handle(),
			Callback:         cq.Data,
		}
		reply := b.handler.Handle(ctx, cmd)
		b.send(ctx, cq.Message.Chat.ID, reply)
	case u.Message != nil && strings.TrimSpace(u.Message.Text) != "":
		m := u.Message
		cmd := bots.Command{
			Platform:         bots.PlatformTelegram,
			ExternalID:       strconv.FormatInt(m.From.ID, 10),
			ExternalUsername: m.From.handle(),
			Text:             m.Text,
		}
		reply := b.handler.Handle(ctx, cmd)
		b.send(ctx, m.Chat.ID, reply)
	}
}

// send delivers a reply, rendering buttons as a one-per-row inline keyboard.
// Telegram errors on malformed Markdown entities, so we send plain text (with
// emphasis markers stripped) to guarantee delivery.
func (b *Bot) send(ctx context.Context, chatID int64, reply bots.Reply) {
	payload := map[string]any{
		"chat_id": chatID,
		"text":    stripMarkers(reply.Text),
	}
	if len(reply.Buttons) > 0 {
		rows := make([][]map[string]string, 0, len(reply.Buttons))
		for _, btn := range reply.Buttons {
			rows = append(rows, []map[string]string{{
				"text":          stripMarkers(btn.Label),
				"callback_data": btn.Data,
			}})
		}
		payload["reply_markup"] = map[string]any{"inline_keyboard": rows}
	}
	if err := b.call(ctx, "sendMessage", payload, nil); err != nil {
		b.logger.Warn("telegram bot: sendMessage failed", "err", err)
	}
}

func (b *Bot) answerCallback(ctx context.Context, id string) error {
	return b.call(ctx, "answerCallbackQuery", map[string]any{"callback_query_id": id}, nil)
}

func (b *Bot) getUpdates(ctx context.Context, offset int64, timeout int) ([]update, error) {
	payload := map[string]any{
		"timeout":         timeout,
		"offset":          offset,
		"allowed_updates": []string{"message", "callback_query"},
	}
	var resp struct {
		OK     bool     `json:"ok"`
		Result []update `json:"result"`
	}
	if err := b.call(ctx, "getUpdates", payload, &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("telegram: getUpdates not ok")
	}
	return resp.Result, nil
}

// call posts a JSON request to a Bot API method and optionally decodes result.
func (b *Bot) call(ctx context.Context, method string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/bot%s/%s", b.apiBase, b.token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram: %s -> %d: %s", method, res.StatusCode, truncate(string(data), 200))
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

// stripMarkers removes the brain's `*emphasis*` markers for plain-text delivery.
func stripMarkers(s string) string { return strings.ReplaceAll(s, "*", "") }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// --- Telegram API types (subset) ----------------------------------------

type update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *message       `json:"message"`
	CallbackQuery *callbackQuery `json:"callback_query"`
}

type message struct {
	Text string `json:"text"`
	From user   `json:"from"`
	Chat chat   `json:"chat"`
}

type callbackQuery struct {
	ID      string   `json:"id"`
	From    user     `json:"from"`
	Data    string   `json:"data"`
	Message *message `json:"message"`
}

type user struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
}

func (u user) handle() string {
	if u.Username != "" {
		return "@" + u.Username
	}
	return u.FirstName
}

type chat struct {
	ID int64 `json:"id"`
}
