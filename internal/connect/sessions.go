package connect

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ActiveSessions for Trakt: it isn't a streaming server.
func (t *traktProvider) ActiveSessions(_ context.Context, _ ProviderSettings) ([]Session, error) {
	return nil, nil
}

// episodeTag formats a SxxEyy tag from season/episode numbers (0 = unknown).
func episodeTag(season, episode int) string {
	if season <= 0 && episode <= 0 {
		return ""
	}
	return fmt.Sprintf("S%02dE%02d", season, episode)
}

// buildFullTitle composes a human-readable title for an episode or movie.
func buildFullTitle(mediaType, title, show string, season, episode int) string {
	if mediaType == "episode" && show != "" {
		tag := episodeTag(season, episode)
		parts := []string{show}
		if tag != "" {
			parts = append(parts, tag)
		}
		if title != "" {
			parts = append(parts, title)
		}
		return strings.Join(parts, " - ")
	}
	return title
}

// ---------- Plex ----------

type plexSessionsResponse struct {
	MediaContainer struct {
		Metadata []plexSession `json:"Metadata"`
	} `json:"MediaContainer"`
}

type plexSession struct {
	SessionKey       string `json:"sessionKey"`
	RatingKey        string `json:"ratingKey"`
	Type             string `json:"type"`
	Title            string `json:"title"`
	GrandparentTitle string `json:"grandparentTitle"`
	ParentIndex      int    `json:"parentIndex"`
	Index            int    `json:"index"`
	ViewOffset       int64  `json:"viewOffset"`
	Duration         int64  `json:"duration"`
	Media            []struct {
		Bitrate int64 `json:"bitrate"`
	} `json:"Media"`
	Session *struct {
		Bandwidth int64 `json:"bandwidth"`
	} `json:"Session"`
	User struct {
		Title string `json:"title"`
	} `json:"User"`
	Player struct {
		Title string `json:"title"`
		State string `json:"state"`
	} `json:"Player"`
	TranscodeSession *json.RawMessage `json:"TranscodeSession"`
}

func (p *plexProvider) ActiveSessions(ctx context.Context, s ProviderSettings) ([]Session, error) {
	base := strings.TrimRight(s.Host, "/")
	req, err := http.NewRequestWithContext(ctx, "GET", base+"/status/sessions", nil)
	if err != nil {
		return nil, fmt.Errorf("plex sessions: %w", err)
	}
	req.Header.Set("X-Plex-Token", s.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("plex sessions: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plex sessions: unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("plex sessions read: %w", err)
	}
	var parsed plexSessionsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("plex sessions parse: %w", err)
	}

	out := make([]Session, 0, len(parsed.MediaContainer.Metadata))
	for _, m := range parsed.MediaContainer.Metadata {
		mediaType := "other"
		switch m.Type {
		case "movie":
			mediaType = "movie"
		case "episode":
			mediaType = "episode"
		}
		state := "playing"
		if m.Player.State == "paused" {
			state = "paused"
		}
		// Prefer the live streaming bandwidth Plex reports for the session;
		// fall back to the source media bitrate.
		var bitrate int64
		if m.Session != nil && m.Session.Bandwidth > 0 {
			bitrate = m.Session.Bandwidth
		} else if len(m.Media) > 0 {
			bitrate = m.Media[0].Bitrate
		}
		out = append(out, Session{
			SessionKey:       m.SessionKey,
			MediaID:          m.RatingKey,
			User:             m.User.Title,
			MediaType:        mediaType,
			Title:            m.Title,
			GrandparentTitle: m.GrandparentTitle,
			FullTitle:        buildFullTitle(mediaType, m.Title, m.GrandparentTitle, m.ParentIndex, m.Index),
			Device:           m.Player.Title,
			State:            state,
			PositionMs:       m.ViewOffset,
			DurationMs:       m.Duration,
			Transcode:        m.TranscodeSession != nil,
			BitrateKbps:      bitrate,
		})
	}
	return out, nil
}

// ---------- Emby / Jellyfin (shared JSON shape) ----------

type embySession struct {
	ID             string `json:"Id"`
	UserName       string `json:"UserName"`
	DeviceName     string `json:"DeviceName"`
	NowPlayingItem *struct {
		Name              string `json:"Name"`
		SeriesName        string `json:"SeriesName"`
		Type              string `json:"Type"`
		ID                string `json:"Id"`
		RunTimeTicks      int64  `json:"RunTimeTicks"`
		ParentIndexNumber int    `json:"ParentIndexNumber"`
		IndexNumber       int    `json:"IndexNumber"`
	} `json:"NowPlayingItem"`
	PlayState *struct {
		PositionTicks int64  `json:"PositionTicks"`
		IsPaused      bool   `json:"IsPaused"`
		PlayMethod    string `json:"PlayMethod"`
	} `json:"PlayState"`
	TranscodingInfo *struct {
		Bitrate int64 `json:"Bitrate"` // bits per second
	} `json:"TranscodingInfo"`
}

// ticksToMs converts .NET 100ns ticks to milliseconds.
func ticksToMs(ticks int64) int64 { return ticks / 10000 }

func parseEmbySessions(body []byte) ([]Session, error) {
	var raw []embySession
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	out := make([]Session, 0, len(raw))
	for _, s := range raw {
		if s.NowPlayingItem == nil {
			continue
		}
		item := s.NowPlayingItem
		mediaType := "other"
		switch item.Type {
		case "Movie":
			mediaType = "movie"
		case "Episode":
			mediaType = "episode"
		}
		state := "playing"
		transcode := false
		var pos int64
		if s.PlayState != nil {
			if s.PlayState.IsPaused {
				state = "paused"
			}
			transcode = strings.EqualFold(s.PlayState.PlayMethod, "Transcode")
			pos = ticksToMs(s.PlayState.PositionTicks)
		}
		// Emby/Jellyfin only report a bitrate while transcoding (bits/sec).
		var bitrate int64
		if s.TranscodingInfo != nil && s.TranscodingInfo.Bitrate > 0 {
			bitrate = s.TranscodingInfo.Bitrate / 1000
		}
		out = append(out, Session{
			SessionKey:       s.ID,
			MediaID:          item.ID,
			User:             s.UserName,
			MediaType:        mediaType,
			Title:            item.Name,
			GrandparentTitle: item.SeriesName,
			FullTitle:        buildFullTitle(mediaType, item.Name, item.SeriesName, item.ParentIndexNumber, item.IndexNumber),
			Device:           s.DeviceName,
			State:            state,
			PositionMs:       pos,
			DurationMs:       ticksToMs(item.RunTimeTicks),
			Transcode:        transcode,
			BitrateKbps:      bitrate,
		})
	}
	return out, nil
}

func fetchEmbyStyleSessions(ctx context.Context, base string, setAuth func(*http.Request)) ([]Session, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(base, "/")+"/Sessions", nil)
	if err != nil {
		return nil, err
	}
	setAuth(req)
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	return parseEmbySessions(body)
}

func (p *embyProvider) ActiveSessions(ctx context.Context, s ProviderSettings) ([]Session, error) {
	sessions, err := fetchEmbyStyleSessions(ctx, s.Host, func(r *http.Request) {
		r.Header.Set("X-Emby-Token", s.APIKey)
	})
	if err != nil {
		return nil, fmt.Errorf("emby sessions: %w", err)
	}
	return sessions, nil
}

func (p *jellyfinProvider) ActiveSessions(ctx context.Context, s ProviderSettings) ([]Session, error) {
	sessions, err := fetchEmbyStyleSessions(ctx, s.Host, func(r *http.Request) {
		r.Header.Set("Authorization", fmt.Sprintf(`MediaBrowser Token="%s"`, s.APIKey))
	})
	if err != nil {
		return nil, fmt.Errorf("jellyfin sessions: %w", err)
	}
	return sessions, nil
}
