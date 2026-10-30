package calendar

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	_ "time/tzdata" // Static/NAS builds must recognize browser IANA timezones too.

	"github.com/go-chi/chi/v5"

	"github.com/ebenderooock/loom/internal/airing"
	"github.com/ebenderooock/loom/internal/storage"
)

// Event represents a unified calendar event (movie release or episode air date).
type Event struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Type         string `json:"type"`
	ReleaseType  string `json:"releaseType,omitempty"`
	Date         string `json:"date"`
	AirDateUTC   string `json:"airDateUtc,omitempty"`
	Status       string `json:"status"`
	Monitored    bool   `json:"monitored"`
	SeriesID     string `json:"seriesId,omitempty"`
	Year         int    `json:"year,omitempty"`
	SeriesTitle  string `json:"seriesTitle,omitempty"`
	Season       int    `json:"season,omitempty"`
	Episode      int    `json:"episode,omitempty"`
	EpisodeTitle string `json:"episodeTitle,omitempty"`
	Artist       string `json:"artist,omitempty"`
}

// Handler serves the calendar API.
type Handler struct {
	db        *sql.DB
	postgres  bool
	workflows WorkflowStatusSource
}

// WorkflowStatusSource supplies acquisition states independently of monitoring.
type WorkflowStatusSource interface {
	ActiveMediaStates(ctx context.Context, mediaType string, ids []string) (map[string]string, error)
}

// NewHandler creates a new calendar handler.
func NewHandler(db *sql.DB, engine ...string) *Handler {
	postgres := len(engine) > 0 && engine[0] == "postgres"
	return &Handler{db: db, postgres: postgres}
}

// Router returns a chi.Router with the calendar endpoint.
func Router(db *sql.DB, sources ...WorkflowStatusSource) chi.Router {
	return RouterForEngine(db, "", sources...)
}

// RouterForEngine is Router with SQL placeholders rebound for the given
// storage engine ("postgres" or "sqlite").
func RouterForEngine(db *sql.DB, engine string, sources ...WorkflowStatusSource) chi.Router {
	h := NewHandler(db, engine)
	if len(sources) > 0 {
		h.workflows = sources[0]
	}
	r := chi.NewRouter()
	r.Get("/", h.list)
	return r
}

func (h *Handler) queryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return h.db.QueryContext(ctx, storage.RebindQuery(query, h.postgres), args...)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	// The browser supplies its IANA timezone; never infer it from the server.
	loc := time.UTC
	if zone := r.URL.Query().Get("timezone"); zone != "" {
		var err error
		loc, err = time.LoadLocation(zone)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid timezone")
			return
		}
	}
	now = now.In(loc)
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	if startStr == "" {
		startStr = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	}
	if endStr == "" {
		endStr = time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	}

	// Validate date formats
	if _, err := time.Parse("2006-01-02", startStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid start date format, expected YYYY-MM-DD")
		return
	}
	if _, err := time.Parse("2006-01-02", endStr); err != nil {
		writeError(w, http.StatusBadRequest, "invalid end date format, expected YYYY-MM-DD")
		return
	}
	if startStr > endStr {
		writeError(w, http.StatusBadRequest, "start date must not be after end date")
		return
	}

	events := make([]Event, 0)

	// Query movies with release_date, theatrical_date, or digital_date in range
	movieRows, err := h.queryContext(r.Context(),
		`SELECT id, title, year, release_date, status, theatrical_date, digital_date, monitoring_status FROM movies
		 WHERE deleted_at IS NULL AND (
		   (release_date >= ? AND release_date <= ?) OR
		   (theatrical_date != '' AND theatrical_date >= ? AND theatrical_date <= ?) OR
		   (digital_date != '' AND digital_date >= ? AND digital_date <= ?)
		 )`,
		startStr, endStr, startStr, endStr, startStr, endStr)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("query movies: %v", err))
		return
	}
	defer movieRows.Close()

	for movieRows.Next() {
		var id, title, releaseDate, status, theatricalDate, digitalDate, monitoring string
		var year int
		if err := movieRows.Scan(&id, &title, &year, &releaseDate, &status, &theatricalDate, &digitalDate, &monitoring); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("scan movie: %v", err))
			return
		}
		calStatus := status
		switch status {
		case "available_wrong_quality", "available_right_quality", "available_higher_quality":
			calStatus = "downloaded"
		case "storing":
			calStatus = "importing"
		}
		appendMovieEvent := func(date, releaseType string) {
			if date != "" && date >= startStr && date <= endStr {
				events = append(events, Event{
					ID: id, Title: title, Type: "movie",
					ReleaseType: releaseType, Date: date,
					Status: calStatus, Year: year, Monitored: monitoring == "monitored",
				})
			}
		}
		appendMovieEvent(releaseDate, "release")
		appendMovieEvent(theatricalDate, "theatrical")
		appendMovieEvent(digitalDate, "digital")
	}
	if err := movieRows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("iterate movies: %v", err))
		return
	}

	// Expand to UTC instants of the requested local days, including DST.
	startDay, _ := time.ParseInLocation("2006-01-02", startStr, loc)
	endDay, _ := time.ParseInLocation("2006-01-02", endStr, loc)
	rangeStart := startDay.UTC().Format(time.RFC3339)
	rangeEnd := endDay.AddDate(0, 0, 1).UTC().Format(time.RFC3339)

	// Date-only rows retain their original calendar date, without conversion.
	epRows, err := h.queryContext(r.Context(),
		`SELECT e.id, e.title, e.air_date, e.episode_number, e.has_file, e.air_date_utc,
		        s.title, s.id, sea.season_number, e.monitored
		 FROM episodes e
		 JOIN series s ON s.id = e.series_id
		 JOIN seasons sea ON sea.id = e.season_id
		 WHERE (e.air_date_utc = '' AND e.air_date >= ? AND e.air_date <= ?)
		    OR (e.air_date_utc != '' AND julianday(e.air_date_utc) >= julianday(?)
		        AND julianday(e.air_date_utc) < julianday(?))`,
		startStr, endStr, rangeStart, rangeEnd)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("query episodes: %v", err))
		return
	}
	defer epRows.Close()

	for epRows.Next() {
		var epID, epTitle, airDate, airDateUTC, seriesTitle, seriesID string
		var epNum, seasonNum int
		var hasFile, monitored bool
		if err := epRows.Scan(&epID, &epTitle, &airDate, &epNum, &hasFile, &airDateUTC, &seriesTitle, &seriesID, &seasonNum, &monitored); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("scan episode: %v", err))
			return
		}
		calStatus := "missing"
		if hasFile {
			calStatus = "downloaded"
		} else if !airing.HasAired(airDate, airDateUTC, now) {
			calStatus = "unaired"
		}
		if t, err := time.Parse(time.RFC3339, airDateUTC); err == nil {
			airDate = t.In(loc).Format("2006-01-02")
		}
		displayTitle := fmt.Sprintf("%s - S%02dE%02d", seriesTitle, seasonNum, epNum)
		if epTitle != "" {
			displayTitle += " - " + epTitle
		}
		events = append(events, Event{
			ID:           epID,
			Title:        displayTitle,
			Type:         "episode",
			Date:         airDate,
			AirDateUTC:   airDateUTC,
			Status:       calStatus,
			Monitored:    monitored,
			SeriesID:     seriesID,
			SeriesTitle:  seriesTitle,
			Season:       seasonNum,
			Episode:      epNum,
			EpisodeTitle: epTitle,
		})
	}
	if err := epRows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("iterate episodes: %v", err))
		return
	}

	// Query monitored albums with a release_date in range (music). The albums
	// tables only contain rows once the music capability is used, so this is a
	// no-op for installs that never enabled it.
	albumRows, err := h.queryContext(r.Context(),
		`SELECT al.id, al.title, al.release_date, ar.name, al.monitored,
		        EXISTS (SELECT 1 FROM track_files tf WHERE tf.album_id = al.id AND tf.deleted_at IS NULL)
		 FROM albums al
		 JOIN artists ar ON ar.id = al.artist_id
		 WHERE al.deleted_at IS NULL AND al.release_date != ''
		   AND al.release_date >= ? AND al.release_date <= ?`,
		startStr, endStr)
	if err == nil {
		defer albumRows.Close()
		for albumRows.Next() {
			var id, title, releaseDate, artist string
			var hasFile, monitored bool
			if err := albumRows.Scan(&id, &title, &releaseDate, &artist, &monitored, &hasFile); err != nil {
				writeError(w, http.StatusInternalServerError, fmt.Sprintf("scan album: %v", err))
				return
			}
			calStatus := "missing"
			if hasFile {
				calStatus = "downloaded"
			}
			events = append(events, Event{
				ID:        id,
				Title:     fmt.Sprintf("%s - %s", artist, title),
				Type:      "album",
				Date:      releaseDate,
				Status:    calStatus,
				Monitored: monitored,
				Artist:    artist,
			})
		}
		if err := albumRows.Err(); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("iterate albums: %v", err))
			return
		}
	}

	if h.workflows != nil {
		for _, mediaType := range []string{"movie", "episode"} {
			var ids []string
			for _, event := range events {
				if event.Type == mediaType {
					ids = append(ids, event.ID)
				}
			}
			states, err := h.workflows.ActiveMediaStates(r.Context(), mediaType, ids)
			if err != nil {
				writeError(w, http.StatusInternalServerError, fmt.Sprintf("query workflow status: %v", err))
				return
			}
			for i := range events {
				event := &events[i]
				if event.Type != mediaType || event.Status == "downloaded" {
					continue
				}
				if state := states[event.ID]; state != "" {
					switch state {
					case "post_download", "importing", "cleaning_up":
						event.Status = "importing"
					default:
						event.Status = state
					}
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, events)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    http.StatusText(status),
			"message": msg,
		},
	})
}
