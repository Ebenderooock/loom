package calendar

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// Event represents a unified calendar event (movie release or episode air date).
type Event struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Type         string `json:"type"`
	ReleaseType  string `json:"releaseType,omitempty"`
	Date         string `json:"date"`
	Status       string `json:"status"`
	Year         int    `json:"year,omitempty"`
	SeriesTitle  string `json:"seriesTitle,omitempty"`
	Season       int    `json:"season,omitempty"`
	Episode      int    `json:"episode,omitempty"`
	EpisodeTitle string `json:"episodeTitle,omitempty"`
}

// Handler serves the calendar API.
type Handler struct {
	db *sql.DB
}

// NewHandler creates a new calendar handler.
func NewHandler(db *sql.DB) *Handler {
	return &Handler{db: db}
}

// Router returns a chi.Router with the calendar endpoint.
func Router(db *sql.DB) chi.Router {
	h := NewHandler(db)
	r := chi.NewRouter()
	r.Get("/", h.list)
	return r
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
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

	events := make([]Event, 0)

	// Query movies with release_date, theatrical_date, or digital_date in range
	movieRows, err := h.db.QueryContext(r.Context(),
		`SELECT id, title, year, release_date, status, theatrical_date, digital_date FROM movies
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
		var id, title, releaseDate, status, theatricalDate, digitalDate string
		var year int
		if err := movieRows.Scan(&id, &title, &year, &releaseDate, &status, &theatricalDate, &digitalDate); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("scan movie: %v", err))
			return
		}
		calStatus := "missing"
		if status != "missing" && status != "unreleased" {
			calStatus = "downloaded"
		}
		appendMovieEvent := func(date, releaseType string) {
			if date != "" {
				events = append(events, Event{
					ID: id, Title: title, Type: "movie",
					ReleaseType: releaseType, Date: date,
					Status: calStatus, Year: year,
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

	// Query episodes with air_date in range
	epRows, err := h.db.QueryContext(r.Context(),
		`SELECT e.id, e.title, e.air_date, e.episode_number, e.has_file,
		        s.title, s.id, sea.season_number
		 FROM episodes e
		 JOIN series s ON s.id = e.series_id
		 JOIN seasons sea ON sea.id = e.season_id
		 WHERE e.air_date >= ? AND e.air_date <= ?`,
		startStr, endStr)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("query episodes: %v", err))
		return
	}
	defer epRows.Close()

	for epRows.Next() {
		var epID, epTitle, airDate, seriesTitle, seriesID string
		var epNum, seasonNum int
		var hasFile bool
		if err := epRows.Scan(&epID, &epTitle, &airDate, &epNum, &hasFile, &seriesTitle, &seriesID, &seasonNum); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("scan episode: %v", err))
			return
		}
		calStatus := "missing"
		if hasFile {
			calStatus = "downloaded"
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
			Status:       calStatus,
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
