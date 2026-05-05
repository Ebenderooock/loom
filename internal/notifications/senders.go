package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/smtp"
	"net/url"
	"time"
)

// Sender is the interface each notification backend implements.
type Sender interface {
	Send(ctx context.Context, n Notification, s ConnectionSettings) error
}

// senderFor returns the appropriate sender for a connection type.
func senderFor(ct ConnectionType) Sender {
	switch ct {
	case ConnectionTypeDiscord:
		return &DiscordSender{}
	case ConnectionTypeWebhook:
		return &WebhookSender{}
	case ConnectionTypeSlack:
		return &SlackSender{}
	case ConnectionTypeEmail:
		return &EmailSender{}
	case ConnectionTypeTelegram:
		return &TelegramSender{}
	case ConnectionTypeGotify:
		return &GotifySender{}
	case ConnectionTypePushover:
		return &PushoverSender{}
	case ConnectionTypeNtfy:
		return &NtfySender{}
	case ConnectionTypeApprise:
		return &AppriseSender{}
	default:
		return &WebhookSender{}
	}
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

func postJSON(ctx context.Context, url string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// DiscordSender posts embeds to a Discord webhook URL.
type DiscordSender struct{}

func (d *DiscordSender) Send(ctx context.Context, n Notification, s ConnectionSettings) error {
	if s.WebhookURL == "" {
		return fmt.Errorf("discord: webhook_url is required")
	}
	payload := map[string]any{
		"embeds": []map[string]any{
			{
				"title":       n.Title,
				"description": n.Message,
				"color":       3447003, // blue
			},
		},
	}
	return postJSON(ctx, s.WebhookURL, payload)
}

// WebhookSender posts the notification as JSON to a generic webhook URL.
type WebhookSender struct{}

func (w *WebhookSender) Send(ctx context.Context, n Notification, s ConnectionSettings) error {
	if s.WebhookURL == "" {
		return fmt.Errorf("webhook: webhook_url is required")
	}
	payload := map[string]any{
		"event":   string(n.EventType),
		"title":   n.Title,
		"message": n.Message,
		"data":    n.Data,
	}
	return postJSON(ctx, s.WebhookURL, payload)
}

// SlackSender posts a message to a Slack webhook URL.
type SlackSender struct{}

func (sl *SlackSender) Send(ctx context.Context, n Notification, s ConnectionSettings) error {
	if s.WebhookURL == "" {
		return fmt.Errorf("slack: webhook_url is required")
	}
	payload := map[string]any{
		"blocks": []map[string]any{
			{
				"type": "header",
				"text": map[string]any{
					"type": "plain_text",
					"text": n.Title,
				},
			},
			{
				"type": "section",
				"text": map[string]any{
					"type": "mrkdwn",
					"text": n.Message,
				},
			},
		},
	}
	return postJSON(ctx, s.WebhookURL, payload)
}

// EmailSender sends notifications via SMTP.
type EmailSender struct{}

func (e *EmailSender) Send(ctx context.Context, n Notification, s ConnectionSettings) error {
	if s.Host == "" || s.From == "" || s.To == "" {
		return fmt.Errorf("email: host, from, and to are required")
	}
	port := s.Port
	if port == 0 {
		port = 587
	}
	addr := fmt.Sprintf("%s:%d", s.Host, port)
	subject := n.Title
	body := fmt.Sprintf("Subject: %s\r\nFrom: %s\r\nTo: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		subject, s.From, s.To, n.Message)

	var auth smtp.Auth
	if s.Username != "" {
		auth = smtp.PlainAuth("", s.Username, s.Password, s.Host)
	}
	return smtp.SendMail(addr, auth, s.From, []string{s.To}, []byte(body))
}

// TelegramSender posts to the Telegram Bot API.
type TelegramSender struct{}

func (t *TelegramSender) Send(ctx context.Context, n Notification, s ConnectionSettings) error {
	if s.BotToken == "" || s.ChatID == "" {
		return fmt.Errorf("telegram: bot_token and chat_id are required")
	}
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", s.BotToken)
	payload := map[string]any{
		"chat_id":    s.ChatID,
		"text":       fmt.Sprintf("*%s*\n%s", n.Title, n.Message),
		"parse_mode": "Markdown",
	}
	return postJSON(ctx, apiURL, payload)
}

// GotifySender posts to a Gotify server.
type GotifySender struct{}

func (g *GotifySender) Send(ctx context.Context, n Notification, s ConnectionSettings) error {
	if s.ServerURL == "" || s.APIKey == "" {
		return fmt.Errorf("gotify: server_url and api_key are required")
	}
	apiURL := fmt.Sprintf("%s/message?token=%s", s.ServerURL, url.QueryEscape(s.APIKey))
	payload := map[string]any{
		"title":   n.Title,
		"message": n.Message,
	}
	return postJSON(ctx, apiURL, payload)
}

// PushoverSender posts to the Pushover API.
type PushoverSender struct{}

func (p *PushoverSender) Send(ctx context.Context, n Notification, s ConnectionSettings) error {
	if s.APIKey == "" || s.UserKey == "" {
		return fmt.Errorf("pushover: api_key and user_key are required")
	}
	payload := map[string]any{
		"token":   s.APIKey,
		"user":    s.UserKey,
		"title":   n.Title,
		"message": n.Message,
	}
	return postJSON(ctx, "https://api.pushover.net/1/messages.json", payload)
}

// AppriseSender posts to an Apprise API endpoint.
type AppriseSender struct{}

func (a *AppriseSender) Send(ctx context.Context, n Notification, s ConnectionSettings) error {
	if s.ServerURL == "" {
		return fmt.Errorf("apprise: server_url is required")
	}
	apiURL := fmt.Sprintf("%s/notify", s.ServerURL)
	payload := map[string]any{
		"title": n.Title,
		"body":  n.Message,
		"type":  "info",
	}
	return postJSON(ctx, apiURL, payload)
}

// NtfySender publishes to a ntfy topic.
type NtfySender struct{}

func (nt *NtfySender) Send(ctx context.Context, n Notification, s ConnectionSettings) error {
	topicURL := s.ServerURL
	if topicURL == "" {
		return fmt.Errorf("ntfy: server_url (topic URL) is required")
	}
	payload := map[string]any{
		"title":   n.Title,
		"message": n.Message,
		"tags":    []string{"loudspeaker"},
	}
	if s.Topic != "" {
		payload["topic"] = s.Topic
	}
	return postJSON(ctx, topicURL, payload)
}
