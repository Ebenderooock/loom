package series

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Service defines business logic for the series module.
type Service interface {
	ListSeries(ctx context.Context) ([]*Series, error)
	GetSeries(ctx context.Context, id string) (*Series, error)
	AddSeries(ctx context.Context, req *AddSeriesRequest) (*Series, error)
	UpdateSeries(ctx context.Context, s *Series) error
	DeleteSeries(ctx context.Context, id string) error
	SetMonitoringStatus(ctx context.Context, seriesID string, status MonitoringStatus) error
	RefreshSeries(ctx context.Context, id string) error

	ListSeasons(ctx context.Context, seriesID string) ([]*Season, error)
	ListEpisodes(ctx context.Context, seriesID string, seasonNum *int) ([]*Episode, error)
	GetEpisode(ctx context.Context, id string) (*Episode, error)
	UpdateEpisode(ctx context.Context, e *Episode) error
	CreateEpisodeFile(ctx context.Context, f *EpisodeFile) error

	GetCredits(ctx context.Context, seriesID string) ([]*SeriesCredit, error)
	GetEpisodeStats(ctx context.Context, seriesID string) (*EpisodeStats, error)
	GetAllEpisodeStats(ctx context.Context) (map[string]*EpisodeStats, error)
	GetSeasonEpisodeStats(ctx context.Context, seriesID string) (map[string]*EpisodeStats, error)

	SearchTMDB(ctx context.Context, query string) ([]map[string]interface{}, error)
	LookupTMDB(ctx context.Context, tmdbID string) (map[string]interface{}, error)
}

type service struct {
	repo       Repository
	tmdbAPIKey string
	httpClient *http.Client
}

// NewService creates a new series Service.
func NewService(repo Repository, tmdbAPIKey string) Service {
	return &service{
		repo:       repo,
		tmdbAPIKey: tmdbAPIKey,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *service) ListSeries(ctx context.Context) ([]*Series, error) {
	return s.repo.ListSeries(ctx)
}

func (s *service) GetSeries(ctx context.Context, id string) (*Series, error) {
	if id == "" {
		return nil, fmt.Errorf("series: ID required")
	}

	sr, err := s.repo.GetSeries(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("series: get: %w", err)
	}

	// Populate seasons and episodes
	seasons, err := s.repo.ListSeasons(ctx, id)
	if err == nil {
		sr.Seasons = seasons
	}
	episodes, err := s.repo.ListEpisodes(ctx, id, nil)
	if err == nil {
		sr.Episodes = episodes
	}

	return sr, nil
}

func (s *service) AddSeries(ctx context.Context, req *AddSeriesRequest) (*Series, error) {
	if req.TMDBID == "" {
		return nil, fmt.Errorf("series: tmdb_id required")
	}

	// Fetch details from TMDB
	details, err := s.fetchTMDBDetails(ctx, req.TMDBID)
	if err != nil {
		return nil, fmt.Errorf("series: tmdb lookup: %w", err)
	}

	title := getString(details, "name")
	if title == "" {
		title = getString(details, "original_name")
	}

	year := 0
	if firstAir := getString(details, "first_air_date"); len(firstAir) >= 4 {
		year, _ = strconv.Atoi(firstAir[:4])
	}

	slug := slugify(title)
	if year > 0 {
		slug = slug + "-" + strconv.Itoa(year)
	}

	// Extract genres
	var genres StringSlice
	if genreList, ok := details["genres"].([]interface{}); ok {
		for _, g := range genreList {
			if gm, ok := g.(map[string]interface{}); ok {
				if name, ok := gm["name"].(string); ok {
					genres = append(genres, name)
				}
			}
		}
	}

	// Network
	network := ""
	if networks, ok := details["networks"].([]interface{}); ok && len(networks) > 0 {
		if nm, ok := networks[0].(map[string]interface{}); ok {
			network = getString(nm, "name")
		}
	}

	// Status mapping
	tmdbStatus := strings.ToLower(getString(details, "status"))
	seriesStatus := StatusContinuing
	switch {
	case strings.Contains(tmdbStatus, "ended"):
		seriesStatus = StatusEnded
	case strings.Contains(tmdbStatus, "cancel"):
		seriesStatus = StatusCancelled
	case strings.Contains(tmdbStatus, "plan"), strings.Contains(tmdbStatus, "pilot"):
		seriesStatus = StatusUpcoming
	}

	tmdbIDStr := req.TMDBID
	monStatus := MonitoringStatus(req.MonitoringStatus)
	if monStatus == "" {
		monStatus = MonitoringAll
	}

	seriesType := SeriesType(req.SeriesType)
	if seriesType == "" {
		seriesType = TypeStandard
	}

	seasonFolder := true
	if req.SeasonFolder != nil {
		seasonFolder = *req.SeasonFolder
	}

	nowt := now()
	sr := &Series{
		ID:               slug,
		Title:            title,
		Year:             year,
		TMDBID:           &tmdbIDStr,
		Overview:         getString(details, "overview"),
		Genres:           genres,
		Runtime:          getInt(details, "episode_run_time"),
		Rating:           getFloat(details, "vote_average"),
		BackdropPath:     getString(details, "backdrop_path"),
		PosterPath:       getString(details, "poster_path"),
		Network:          network,
		Status:           seriesStatus,
		SeriesType:       seriesType,
		MetadataProvider: "tmdb",
		QualityProfileID: req.QualityProfileID,
		LibraryID:        req.LibraryID,
		MonitoringStatus: monStatus,
		SeasonFolder:     seasonFolder,
		ReleaseDate:      getString(details, "first_air_date"),
		CreatedAt:        nowt,
		UpdatedAt:        nowt,
	}

	if err := s.repo.CreateSeries(ctx, sr); err != nil {
		return nil, fmt.Errorf("series: create: %w", err)
	}

	// Create seasons and episodes from TMDB data
	if seasons, ok := details["seasons"].([]interface{}); ok {
		for _, sRaw := range seasons {
			sm, ok := sRaw.(map[string]interface{})
			if !ok {
				continue
			}

			seasonNum := getInt(sm, "season_number")
			seasonID := fmt.Sprintf("%s-s%02d", slug, seasonNum)

			season := &Season{
				ID:           seasonID,
				SeriesID:     sr.ID,
				SeasonNumber: seasonNum,
				Title:        getString(sm, "name"),
				Overview:     getString(sm, "overview"),
				PosterPath:   getString(sm, "poster_path"),
				Monitored:    true,
				EpisodeCount: getInt(sm, "episode_count"),
				CreatedAt:    nowt,
				UpdatedAt:    nowt,
			}
			if err := s.repo.CreateSeason(ctx, season); err != nil {
				continue
			}

			// Fetch individual season episodes from TMDB
			s.fetchAndStoreEpisodes(ctx, req.TMDBID, seasonNum, sr.ID, seasonID, nowt)
		}
	}

	// Save credits
	s.fetchAndStoreCredits(ctx, details, sr.ID)

	return sr, nil
}

func (s *service) UpdateSeries(ctx context.Context, sr *Series) error {
	if sr == nil {
		return fmt.Errorf("series: series required")
	}
	if sr.ID == "" {
		return fmt.Errorf("series: ID required")
	}
	sr.UpdatedAt = now()
	return s.repo.UpdateSeries(ctx, sr)
}

func (s *service) DeleteSeries(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("series: ID required")
	}
	return s.repo.DeleteSeries(ctx, id)
}

func (s *service) SetMonitoringStatus(ctx context.Context, seriesID string, status MonitoringStatus) error {
	if seriesID == "" {
		return fmt.Errorf("series: ID required")
	}

	sr, err := s.repo.GetSeries(ctx, seriesID)
	if err != nil {
		return fmt.Errorf("series: get: %w", err)
	}

	sr.MonitoringStatus = status
	sr.UpdatedAt = now()
	return s.repo.UpdateSeries(ctx, sr)
}

func (s *service) RefreshSeries(ctx context.Context, id string) error {
	sr, err := s.repo.GetSeries(ctx, id)
	if err != nil {
		return fmt.Errorf("series: get: %w", err)
	}
	if sr.TMDBID == nil || *sr.TMDBID == "" {
		return fmt.Errorf("series: no TMDB ID for refresh")
	}

	details, err := s.fetchTMDBDetails(ctx, *sr.TMDBID)
	if err != nil {
		return fmt.Errorf("series: tmdb refresh: %w", err)
	}

	// Update series fields from TMDB
	sr.Title = getString(details, "name")
	sr.Overview = getString(details, "overview")
	sr.Rating = getFloat(details, "vote_average")
	sr.BackdropPath = getString(details, "backdrop_path")
	sr.PosterPath = getString(details, "poster_path")
	sr.Runtime = getInt(details, "episode_run_time")

	// Extract genres
	var genres StringSlice
	if genreList, ok := details["genres"].([]interface{}); ok {
		for _, g := range genreList {
			if gm, ok := g.(map[string]interface{}); ok {
				if name, ok := gm["name"].(string); ok {
					genres = append(genres, name)
				}
			}
		}
	}
	sr.Genres = genres

	// Network
	if networks, ok := details["networks"].([]interface{}); ok && len(networks) > 0 {
		if nm, ok := networks[0].(map[string]interface{}); ok {
			sr.Network = getString(nm, "name")
		}
	}

	nowt := now()
	sr.UpdatedAt = nowt

	tmdbStatus := strings.ToLower(getString(details, "status"))
	switch {
	case strings.Contains(tmdbStatus, "ended"):
		sr.Status = StatusEnded
	case strings.Contains(tmdbStatus, "cancel"):
		sr.Status = StatusCancelled
	case strings.Contains(tmdbStatus, "plan"), strings.Contains(tmdbStatus, "pilot"):
		sr.Status = StatusUpcoming
	default:
		sr.Status = StatusContinuing
	}

	if err := s.repo.UpdateSeries(ctx, sr); err != nil {
		return fmt.Errorf("series: update: %w", err)
	}

	// Delete existing children, then re-create from fresh TMDB data
	if err := s.repo.DeleteEpisodesBySeriesID(ctx, sr.ID); err != nil {
		return fmt.Errorf("series: delete episodes: %w", err)
	}
	if err := s.repo.DeleteSeasonsBySeriesID(ctx, sr.ID); err != nil {
		return fmt.Errorf("series: delete seasons: %w", err)
	}
	if err := s.repo.DeleteCreditsBySeriesID(ctx, sr.ID); err != nil {
		return fmt.Errorf("series: delete credits: %w", err)
	}

	// Re-create seasons and episodes
	if seasons, ok := details["seasons"].([]interface{}); ok {
		for _, sRaw := range seasons {
			sm, ok := sRaw.(map[string]interface{})
			if !ok {
				continue
			}

			seasonNum := getInt(sm, "season_number")
			seasonID := fmt.Sprintf("%s-s%02d", sr.ID, seasonNum)

			season := &Season{
				ID:           seasonID,
				SeriesID:     sr.ID,
				SeasonNumber: seasonNum,
				Title:        getString(sm, "name"),
				Overview:     getString(sm, "overview"),
				PosterPath:   getString(sm, "poster_path"),
				Monitored:    true,
				EpisodeCount: getInt(sm, "episode_count"),
				CreatedAt:    nowt,
				UpdatedAt:    nowt,
			}
			if err := s.repo.CreateSeason(ctx, season); err != nil {
				continue
			}

			s.fetchAndStoreEpisodes(ctx, *sr.TMDBID, seasonNum, sr.ID, seasonID, nowt)
		}
	}

	// Re-create credits
	s.fetchAndStoreCredits(ctx, details, sr.ID)

	return nil
}

func (s *service) ListSeasons(ctx context.Context, seriesID string) ([]*Season, error) {
	return s.repo.ListSeasons(ctx, seriesID)
}

func (s *service) ListEpisodes(ctx context.Context, seriesID string, seasonNum *int) ([]*Episode, error) {
	return s.repo.ListEpisodes(ctx, seriesID, seasonNum)
}

func (s *service) GetEpisode(ctx context.Context, id string) (*Episode, error) {
	return s.repo.GetEpisode(ctx, id)
}

func (s *service) UpdateEpisode(ctx context.Context, e *Episode) error {
	if e == nil {
		return fmt.Errorf("series: episode required")
	}
	e.UpdatedAt = now()
	return s.repo.UpdateEpisode(ctx, e)
}

func (s *service) CreateEpisodeFile(ctx context.Context, f *EpisodeFile) error {
	if f == nil {
		return fmt.Errorf("series: episode file required")
	}
	return s.repo.CreateEpisodeFile(ctx, f)
}

func (s *service) GetCredits(ctx context.Context, seriesID string) ([]*SeriesCredit, error) {
	return s.repo.GetCredits(ctx, seriesID)
}

func (s *service) GetEpisodeStats(ctx context.Context, seriesID string) (*EpisodeStats, error) {
	return s.repo.GetEpisodeStats(ctx, seriesID)
}

func (s *service) GetAllEpisodeStats(ctx context.Context) (map[string]*EpisodeStats, error) {
	return s.repo.GetAllEpisodeStats(ctx)
}

func (s *service) GetSeasonEpisodeStats(ctx context.Context, seriesID string) (map[string]*EpisodeStats, error) {
	return s.repo.GetSeasonEpisodeStats(ctx, seriesID)
}

// TMDB operations

func (s *service) SearchTMDB(ctx context.Context, query string) ([]map[string]interface{}, error) {
	if query == "" {
		return nil, fmt.Errorf("series: search query required")
	}
	if s.tmdbAPIKey == "" {
		return nil, fmt.Errorf("series: TMDB API key not configured")
	}

	u := fmt.Sprintf("https://api.themoviedb.org/3/search/tv?query=%s", url.QueryEscape(query))
	body, err := s.tmdbGet(ctx, u)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Results []map[string]interface{} `json:"results"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("series: parse tmdb response: %w", err)
	}
	return resp.Results, nil
}

func (s *service) LookupTMDB(ctx context.Context, tmdbID string) (map[string]interface{}, error) {
	if tmdbID == "" {
		return nil, fmt.Errorf("series: tmdb_id required")
	}
	return s.fetchTMDBDetails(ctx, tmdbID)
}

func (s *service) fetchTMDBDetails(ctx context.Context, tmdbID string) (map[string]interface{}, error) {
	u := fmt.Sprintf("https://api.themoviedb.org/3/tv/%s?append_to_response=credits", tmdbID)
	body, err := s.tmdbGet(ctx, u)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("series: parse tmdb response: %w", err)
	}
	return result, nil
}

func (s *service) tmdbGet(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("series: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.tmdbAPIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("series: tmdb request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("series: read tmdb response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("series: tmdb returned %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func (s *service) fetchAndStoreEpisodes(ctx context.Context, tmdbID string, seasonNum int, seriesID, seasonID string, ts time.Time) {
	u := fmt.Sprintf("https://api.themoviedb.org/3/tv/%s/season/%d", tmdbID, seasonNum)
	body, err := s.tmdbGet(ctx, u)
	if err != nil {
		return
	}

	var seasonData struct {
		Episodes []struct {
			EpisodeNumber int     `json:"episode_number"`
			Name          string  `json:"name"`
			Overview      string  `json:"overview"`
			AirDate       string  `json:"air_date"`
			Runtime       int     `json:"runtime"`
			StillPath     string  `json:"still_path"`
		} `json:"episodes"`
	}
	if err := json.Unmarshal(body, &seasonData); err != nil {
		return
	}

	for _, ep := range seasonData.Episodes {
		epID := fmt.Sprintf("%s-e%03d", seasonID, ep.EpisodeNumber)
		episode := &Episode{
			ID:            epID,
			SeriesID:      seriesID,
			SeasonID:      seasonID,
			EpisodeNumber: ep.EpisodeNumber,
			Title:         ep.Name,
			Overview:      ep.Overview,
			AirDate:       ep.AirDate,
			Runtime:       ep.Runtime,
			StillPath:     ep.StillPath,
			Monitored:     true,
			HasFile:       false,
			CreatedAt:     ts,
			UpdatedAt:     ts,
		}
		_ = s.repo.CreateEpisode(ctx, episode)
	}
}

func (s *service) fetchAndStoreCredits(ctx context.Context, details map[string]interface{}, seriesID string) {
	creditsRaw, ok := details["credits"]
	if !ok {
		return
	}
	creditsMap, ok := creditsRaw.(map[string]interface{})
	if !ok {
		return
	}

	var credits []*SeriesCredit
	order := 0

	if cast, ok := creditsMap["cast"].([]interface{}); ok {
		for _, c := range cast {
			cm, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			credits = append(credits, &SeriesCredit{
				SeriesID:      seriesID,
				PersonName:    getString(cm, "name"),
				CharacterName: getString(cm, "character"),
				Role:          "actor",
				ProfilePath:   getString(cm, "profile_path"),
				TMDBPersonID:  getInt(cm, "id"),
				DisplayOrder:  order,
			})
			order++
			if order >= 25 {
				break
			}
		}
	}

	if crew, ok := creditsMap["crew"].([]interface{}); ok {
		for _, c := range crew {
			cm, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			job := strings.ToLower(getString(cm, "job"))
			if job != "director" && job != "creator" && job != "executive producer" {
				continue
			}
			credits = append(credits, &SeriesCredit{
				SeriesID:     seriesID,
				PersonName:   getString(cm, "name"),
				Role:         getString(cm, "job"),
				ProfilePath:  getString(cm, "profile_path"),
				TMDBPersonID: getInt(cm, "id"),
				DisplayOrder: order,
			})
			order++
		}
	}

	_ = s.repo.SaveCredits(ctx, seriesID, credits)
}

// Helpers

func slugify(name string) string {
	s := strings.ToLower(name)
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r == ' ' || r == '-' || r == '_' || r == '.':
			if !prevDash && b.Len() > 0 {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	result := b.String()
	return strings.TrimRight(result, "-")
}

func getString(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func getInt(m map[string]interface{}, key string) int {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case []interface{}:
		// For fields like episode_run_time which is an array
		if len(n) > 0 {
			if f, ok := n[0].(float64); ok {
				return int(f)
			}
		}
		return 0
	default:
		return 0
	}
}

func getFloat(m map[string]interface{}, key string) float64 {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	f, ok := v.(float64)
	if !ok {
		return 0
	}
	return f
}
