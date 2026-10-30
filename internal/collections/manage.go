package collections

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// ErrUnavailable means library write actions were not wired (e.g. early startup).
var ErrUnavailable = errors.New("library actions are not available; retry shortly")

// Option is a library or quality profile choice exposed to bulk previews.
type Option struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	QualityProfileID string `json:"quality_profile_id,omitempty"`
}

// AddRequest adds one exact provider title through the existing media services.
type AddRequest struct {
	MediaType        string
	TMDBID           string
	Title            string
	Year             int
	LibraryID        string
	QualityProfileID string
	Monitored        bool
	Search           bool
}

// Actions performs library writes through the existing movie/series services,
// quality profiles and automatic search engine. Collections never reimplement
// acquisition; they only select exact titles and report per-item outcomes.
type Actions interface {
	QualityProfiles(context.Context) ([]Option, error)
	Libraries(ctx context.Context, mediaType string) ([]Option, error)
	SetQualityProfile(ctx context.Context, mediaType, mediaID, profileID string) error
	// Add returns the local media ID; adding an existing title is idempotent.
	Add(context.Context, AddRequest) (string, error)
	// SearchMissing searches monitored missing media only (series: monitored,
	// aired, missing episodes). It never adds titles.
	SearchMissing(ctx context.Context, mediaType, mediaID string) error
}

// SetActions wires library write actions after the download stack starts.
func (s *Service) SetActions(a Actions) {
	s.jobsMu.Lock()
	defer s.jobsMu.Unlock()
	s.actions = a
}

func (s *Service) libraryActions() (Actions, error) {
	s.jobsMu.Lock()
	defer s.jobsMu.Unlock()
	if s.actions == nil {
		return nil, ErrUnavailable
	}
	return s.actions, nil
}

// Item status and skip reason codes shared by bulk previews and jobs.
const (
	StatusEligible = "eligible"
	StatusSkipped  = "skipped"
	StatusQueued   = "queued"
	StatusRunning  = "running"
	StatusDone     = "done"
	StatusFailed   = "failed"
)

// BulkItem is one member's eligibility or outcome.
type BulkItem struct {
	MemberID         string `json:"member_id"`
	MediaType        string `json:"media_type"`
	Title            string `json:"title"`
	Year             int    `json:"year,omitempty"`
	MediaID          string `json:"media_id,omitempty"`
	Status           string `json:"status"`
	Reason           string `json:"reason,omitempty"`
	Detail           string `json:"detail,omitempty"`
	CurrentProfileID string `json:"current_profile_id,omitempty"`
	NewProfileID     string `json:"new_profile_id,omitempty"`
	MissingEpisodes  int    `json:"missing_episodes,omitempty"`
	key              string
	add              *AddRequest
}

// AddSettings are the shared add settings for one media type.
type AddSettings struct {
	LibraryID        string `json:"library_id,omitempty"`
	QualityProfileID string `json:"quality_profile_id,omitempty"`
	Monitored        *bool  `json:"monitored,omitempty"`
	Search           bool   `json:"search,omitempty"`
}

// BulkPlan is a preview (execute=false) or the started job/result.
type BulkPlan struct {
	Kind      string                 `json:"kind"`
	Items     []BulkItem             `json:"items"`
	Counts    map[string]int         `json:"counts"`
	Settings  map[string]AddSettings `json:"settings,omitempty"`
	Libraries map[string][]Option    `json:"libraries,omitempty"`
	Profiles  []Option               `json:"profiles,omitempty"`
	Job       *Job                   `json:"job,omitempty"`
	Note      string                 `json:"note,omitempty"`
}

// Job is an in-process background bulk operation. Jobs are not persisted:
// after a restart, the preview shows current eligibility and failed items can
// be retried. Active media keys are deduplicated across concurrent jobs.
type Job struct {
	ID           string         `json:"id"`
	Kind         string         `json:"kind"`
	CollectionID string         `json:"collection_id"`
	Status       string         `json:"status"`
	Items        []BulkItem     `json:"items"`
	Counts       map[string]int `json:"counts"`
	StartedAt    string         `json:"started_at"`
	FinishedAt   string         `json:"finished_at,omitempty"`
}

func count(items []BulkItem) map[string]int {
	out := map[string]int{}
	for _, item := range items {
		out[item.Status]++
	}
	return out
}

type selection struct {
	MemberIDs []string `json:"member_ids,omitempty"`
	All       bool     `json:"all,omitempty"`
}

func (sel selection) pick(c *Collection) ([]Member, error) {
	if sel.All == (len(sel.MemberIDs) > 0) || len(sel.MemberIDs) > 5000 {
		return nil, fmt.Errorf("%w: select members or the whole collection", ErrInvalid)
	}
	if sel.All {
		return c.Members, nil
	}
	index := map[string]Member{}
	for _, m := range c.Members {
		index[m.ID] = m
	}
	out, seen := []Member{}, map[string]bool{}
	for _, id := range sel.MemberIDs {
		m, ok := index[id]
		if !ok {
			return nil, fmt.Errorf("%w: unknown collection member", ErrInvalid)
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, m)
		}
	}
	return out, nil
}

func item(m Member) BulkItem {
	return BulkItem{MemberID: m.ID, MediaType: m.MediaType, Title: m.Title, Year: m.Year, MediaID: m.MediaID}
}

func skip(it *BulkItem, reason, detail string) {
	it.Status, it.Reason, it.Detail = StatusSkipped, reason, detail
}

func unreleased(m Member, now time.Time) bool {
	if len(m.ReleaseDate) < 10 {
		return false
	}
	t, err := time.Parse("2006-01-02", m.ReleaseDate[:10])
	return err == nil && t.After(now)
}

func (s *Service) loadDecorated(ctx context.Context, id string) (*Collection, []libraryRecord, error) {
	c, err := s.getForWrite(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	library, err := s.library(ctx)
	if err != nil {
		return nil, nil, err
	}
	if err := decorate(c, library, true); err != nil {
		return nil, nil, err
	}
	return c, library, nil
}

func recordFor(m Member, library []libraryRecord) *libraryRecord {
	if !m.InLibrary {
		return nil
	}
	for i := range library {
		if library[i].member.MediaType == m.MediaType && library[i].member.MediaID == m.MediaID {
			return &library[i]
		}
	}
	return nil
}

func hasOption(options []Option, id string) *Option {
	for i := range options {
		if options[i].ID == id {
			return &options[i]
		}
	}
	return nil
}

func (s *Service) active(key string) bool {
	s.jobsMu.Lock()
	defer s.jobsMu.Unlock()
	_, ok := s.activeKeys[key]
	return ok
}

// startJob claims dedupe keys atomically; items already claimed are skipped.
func (s *Service) startJob(parent context.Context, kind, collectionID string, items []BulkItem, timeout time.Duration, run func(context.Context, *BulkItem) (string, error)) *Job {
	s.jobsMu.Lock()
	if s.jobs == nil {
		s.jobs, s.activeKeys = map[string]*Job{}, map[string]string{}
	}
	job := &Job{ID: uuid.NewString(), Kind: kind, CollectionID: collectionID, Status: StatusRunning, StartedAt: time.Now().UTC().Format(time.RFC3339)}
	for _, it := range items {
		if it.Status != StatusEligible {
			continue
		}
		if _, busy := s.activeKeys[it.key]; busy {
			skip(&it, "already_running", "Another collection job is already handling this title.")
		} else {
			s.activeKeys[it.key] = job.ID
			it.Status = StatusQueued
		}
		job.Items = append(job.Items, it)
	}
	job.Counts = count(job.Items)
	s.jobs[job.ID] = job
	s.jobOrder = append(s.jobOrder, job.ID)
	for len(s.jobOrder) > 50 {
		delete(s.jobs, s.jobOrder[0])
		s.jobOrder = s.jobOrder[1:]
	}
	snapshot := job.copy()
	s.jobsMu.Unlock()
	// Jobs outlive the request that started them but keep its values.
	base := context.WithoutCancel(parent)

	go func() {
		for i := range snapshot.Items {
			s.jobsMu.Lock()
			current := job.Items[i]
			if current.Status != StatusQueued {
				s.jobsMu.Unlock()
				continue
			}
			job.Items[i].Status = StatusRunning
			job.Counts = count(job.Items)
			s.jobsMu.Unlock()

			ctx, cancel := context.WithTimeout(base, timeout)
			mediaID, err := run(ctx, &current)
			cancel()

			s.jobsMu.Lock()
			if err != nil {
				job.Items[i].Status, job.Items[i].Detail = StatusFailed, err.Error()
			} else {
				job.Items[i].Status = StatusDone
				if mediaID != "" {
					job.Items[i].MediaID = mediaID
				}
			}
			delete(s.activeKeys, current.key)
			job.Counts = count(job.Items)
			s.jobsMu.Unlock()
		}
		s.jobsMu.Lock()
		job.Status, job.FinishedAt = StatusDone, time.Now().UTC().Format(time.RFC3339)
		s.jobsMu.Unlock()
	}()
	return snapshot
}

func (j *Job) copy() *Job {
	c := *j
	c.Items = append([]BulkItem{}, j.Items...)
	c.Counts = map[string]int{}
	for k, v := range j.Counts {
		c.Counts[k] = v
	}
	return &c
}

// Job returns a snapshot of a collection's background job.
func (s *Service) Job(collectionID, id string) (*Job, error) {
	s.jobsMu.Lock()
	defer s.jobsMu.Unlock()
	job, ok := s.jobs[id]
	if !ok || job.CollectionID != collectionID {
		return nil, sql.ErrNoRows
	}
	return job.copy(), nil
}

// Jobs lists recent jobs for one collection, newest first.
func (s *Service) Jobs(collectionID string) []*Job {
	s.jobsMu.Lock()
	defer s.jobsMu.Unlock()
	out := []*Job{}
	for i := len(s.jobOrder) - 1; i >= 0; i-- {
		if job := s.jobs[s.jobOrder[i]]; job != nil && job.CollectionID == collectionID {
			out = append(out, job.copy())
		}
	}
	return out
}

type addBody struct {
	selection
	Execute           bool        `json:"execute"`
	IncludeUnreleased bool        `json:"include_unreleased"`
	Movie             AddSettings `json:"movie"`
	Series            AddSettings `json:"series"`
}

// PlanAdd previews or starts adding missing members with shared add settings.
func (s *Service) PlanAdd(ctx context.Context, id string, body addBody) (*BulkPlan, error) {
	actions, err := s.libraryActions()
	if err != nil {
		return nil, err
	}
	c, _, err := s.loadDecorated(ctx, id)
	if err != nil {
		return nil, err
	}
	members, err := body.pick(c)
	if err != nil {
		return nil, err
	}
	profiles, err := actions.QualityProfiles(ctx)
	if err != nil {
		return nil, err
	}
	plan := &BulkPlan{Kind: "add", Items: []BulkItem{}, Settings: map[string]AddSettings{}, Libraries: map[string][]Option{}, Profiles: profiles}
	for mediaType, requested := range map[string]AddSettings{"movie": body.Movie, "series": body.Series} {
		libs, err := actions.Libraries(ctx, mediaType)
		if err != nil {
			return nil, err
		}
		plan.Libraries[mediaType] = libs
		resolved := AddSettings{Search: requested.Search, Monitored: requested.Monitored}
		if resolved.Monitored == nil {
			yes := true
			resolved.Monitored = &yes
		}
		switch {
		case requested.LibraryID != "":
			if hasOption(libs, requested.LibraryID) == nil {
				return nil, fmt.Errorf("%w: choose a %s library", ErrInvalid, mediaType)
			}
			resolved.LibraryID = requested.LibraryID
		case len(libs) == 1:
			resolved.LibraryID = libs[0].ID
		}
		switch {
		case requested.QualityProfileID != "":
			if hasOption(profiles, requested.QualityProfileID) == nil {
				return nil, fmt.Errorf("%w: unknown quality profile", ErrInvalid)
			}
			resolved.QualityProfileID = requested.QualityProfileID
		case c.QualityProfileID != "" && hasOption(profiles, c.QualityProfileID) != nil:
			resolved.QualityProfileID = c.QualityProfileID
		default:
			if lib := hasOption(libs, resolved.LibraryID); lib != nil && hasOption(profiles, lib.QualityProfileID) != nil {
				resolved.QualityProfileID = lib.QualityProfileID
			}
		}
		plan.Settings[mediaType] = resolved
	}
	now := time.Now()
	for _, m := range members {
		it := item(m)
		settings := plan.Settings[m.MediaType]
		it.key = "add:" + m.MediaType + ":tmdb:" + m.ExternalIDs["tmdb"]
		switch {
		case m.Excluded:
			skip(&it, "excluded", "Excluded from this collection.")
		case m.InLibrary:
			skip(&it, "in_library", "Already in the library.")
		case m.ExternalIDs["tmdb"] == "":
			skip(&it, "unresolved", "No TMDB ID; map this title before adding.")
		case unreleased(m, now) && !body.IncludeUnreleased:
			skip(&it, "unreleased", "Not released yet; include unreleased titles to add it.")
		case settings.LibraryID == "":
			skip(&it, "no_library", "Choose a "+map[string]string{"movie": "movie", "series": "show"}[m.MediaType]+" library.")
		case settings.QualityProfileID == "":
			skip(&it, "no_profile", "Choose a quality profile.")
		case s.active(it.key):
			skip(&it, "already_running", "Another collection job is already adding this title.")
		default:
			it.Status, it.NewProfileID = StatusEligible, settings.QualityProfileID
			it.add = &AddRequest{MediaType: m.MediaType, TMDBID: m.ExternalIDs["tmdb"], Title: m.Title, Year: m.Year, LibraryID: settings.LibraryID, QualityProfileID: settings.QualityProfileID, Monitored: *settings.Monitored, Search: settings.Search}
		}
		plan.Items = append(plan.Items, it)
	}
	plan.Counts = count(plan.Items)
	if body.Execute {
		if plan.Counts[StatusEligible] == 0 {
			return nil, fmt.Errorf("%w: no selected titles can be added", ErrInvalid)
		}
		plan.Job = s.startJob(ctx, "add", c.ID, plan.Items, 2*time.Minute, func(ctx context.Context, it *BulkItem) (string, error) {
			return actions.Add(ctx, *it.add)
		})
	}
	return plan, nil
}

type searchBody struct {
	selection
	Execute bool `json:"execute"`
}

// PlanSearch previews or starts searches for monitored missing library titles.
// It never adds titles: members outside the library are reported and skipped.
func (s *Service) PlanSearch(ctx context.Context, id string, body searchBody) (*BulkPlan, error) {
	actions, err := s.libraryActions()
	if err != nil {
		return nil, err
	}
	c, library, err := s.loadDecorated(ctx, id)
	if err != nil {
		return nil, err
	}
	members, err := body.pick(c)
	if err != nil {
		return nil, err
	}
	plan := &BulkPlan{Kind: "search", Items: []BulkItem{}}
	now := time.Now()
	for _, m := range members {
		it := item(m)
		it.key = "search:" + m.MediaType + ":" + m.MediaID
		record := recordFor(m, library)
		switch {
		case m.Excluded:
			skip(&it, "excluded", "Excluded from this collection.")
		case !m.InLibrary || record == nil:
			skip(&it, "not_in_library", "Not in the library; add it first. Search never adds titles.")
		case !m.Monitored:
			skip(&it, "unmonitored", "Not monitored.")
		case m.MediaType == "movie" && m.HasFile != nil && *m.HasFile:
			skip(&it, "has_file", "Already downloaded.")
		case m.MediaType == "movie" && unreleased(record.member, now):
			skip(&it, "unreleased", "Not released yet.")
		default:
			if m.MediaType == "series" {
				missing := 0
				if record.missing != nil {
					if missing, err = record.missing(); err != nil {
						return nil, err
					}
				}
				if missing == 0 {
					skip(&it, "nothing_missing", "No monitored aired episodes are missing.")
					break
				}
				it.MissingEpisodes = missing
			}
			if s.active(it.key) {
				skip(&it, "already_running", "A search is already running for this title.")
				break
			}
			it.Status = StatusEligible
		}
		plan.Items = append(plan.Items, it)
	}
	plan.Counts = count(plan.Items)
	if body.Execute {
		if plan.Counts[StatusEligible] == 0 {
			return nil, fmt.Errorf("%w: no selected titles are eligible for search", ErrInvalid)
		}
		plan.Job = s.startJob(ctx, "search", c.ID, plan.Items, 10*time.Minute, func(ctx context.Context, it *BulkItem) (string, error) {
			return "", actions.SearchMissing(ctx, it.MediaType, it.MediaID)
		})
	}
	return plan, nil
}

type qualityBody struct {
	selection
	Execute          bool   `json:"execute"`
	QualityProfileID string `json:"quality_profile_id,omitempty"`
}

const qualityNote = "Only the selected library titles change. The collection default only applies to new additions; other collections' defaults are unaffected."

// PlanQuality previews or applies a quality profile to existing library titles.
// It only runs on explicit request; the collection default never overwrites
// title profiles implicitly.
func (s *Service) PlanQuality(ctx context.Context, id string, body qualityBody) (*BulkPlan, error) {
	actions, err := s.libraryActions()
	if err != nil {
		return nil, err
	}
	c, library, err := s.loadDecorated(ctx, id)
	if err != nil {
		return nil, err
	}
	members, err := body.pick(c)
	if err != nil {
		return nil, err
	}
	profiles, err := actions.QualityProfiles(ctx)
	if err != nil {
		return nil, err
	}
	target := body.QualityProfileID
	if target == "" {
		target = c.QualityProfileID
	}
	if target == "" || hasOption(profiles, target) == nil {
		return nil, fmt.Errorf("%w: choose a quality profile", ErrInvalid)
	}
	plan := &BulkPlan{Kind: "quality", Items: []BulkItem{}, Profiles: profiles, Note: qualityNote}
	for _, m := range members {
		it := item(m)
		it.NewProfileID = target
		record := recordFor(m, library)
		switch {
		case m.Excluded:
			skip(&it, "excluded", "Excluded from this collection.")
		case record == nil:
			skip(&it, "not_in_library", "Not in the library.")
		case record.profile == target:
			it.CurrentProfileID = record.profile
			skip(&it, "unchanged", "Already uses this profile.")
		default:
			it.CurrentProfileID, it.Status = record.profile, StatusEligible
		}
		plan.Items = append(plan.Items, it)
	}
	if body.Execute {
		for i := range plan.Items {
			it := &plan.Items[i]
			if it.Status != StatusEligible {
				continue
			}
			if err := actions.SetQualityProfile(ctx, it.MediaType, it.MediaID, target); err != nil {
				it.Status, it.Detail = StatusFailed, err.Error()
			} else {
				it.Status = StatusDone
			}
		}
	}
	plan.Counts = count(plan.Items)
	return plan, nil
}

type memberInput struct {
	MediaType   string            `json:"media_type"`
	MediaID     string            `json:"media_id,omitempty"`
	Title       string            `json:"title,omitempty"`
	Year        int               `json:"year,omitempty"`
	PosterPath  string            `json:"poster_path,omitempty"`
	ExternalIDs map[string]string `json:"external_ids,omitempty"`
}

// AddMembers attaches real titles: existing library media (resolved to their
// exact provider IDs) or provider search results. It never creates media.
func (s *Service) AddMembers(ctx context.Context, id string, revision *int, inputs []memberInput) (*Collection, error) {
	if len(inputs) == 0 || len(inputs) > 100 {
		return nil, fmt.Errorf("%w: select between 1 and 100 titles", ErrInvalid)
	}
	c, err := s.getForWrite(ctx, id)
	if err != nil {
		return nil, err
	}
	if revision != nil && *revision != c.Revision {
		return nil, ErrConflict
	}
	var library []libraryRecord
	for _, in := range inputs {
		var m Member
		switch {
		case in.MediaID != "":
			if len(in.ExternalIDs) > 0 || in.Title != "" {
				return nil, fmt.Errorf("%w: send either a library title or a provider title", ErrInvalid)
			}
			if library == nil {
				if library, err = s.library(ctx); err != nil {
					return nil, err
				}
			}
			var found *Member
			for i := range library {
				if library[i].member.MediaType == in.MediaType && library[i].member.MediaID == in.MediaID {
					found = &library[i].member
				}
			}
			if found == nil {
				return nil, fmt.Errorf("%w: library title not found", ErrInvalid)
			}
			if len(found.ExternalIDs) == 0 {
				return nil, fmt.Errorf("%w: %q has no provider IDs; refresh its metadata first", ErrInvalid, found.Title)
			}
			ids := map[string]string{}
			for p, v := range found.ExternalIDs {
				ids[p] = v
			}
			m = Member{ID: uuid.NewString(), MediaType: found.MediaType, Title: found.Title, Year: found.Year, PosterPath: found.PosterPath, Overview: found.Overview, ReleaseDate: found.ReleaseDate, ExternalIDs: ids}
			if err := m.Validate(); err != nil {
				return nil, err
			}
		default:
			ids := map[string]string{}
			for p, v := range in.ExternalIDs {
				ids[p] = v
			}
			m = Member{MediaType: in.MediaType, Title: strings.TrimSpace(in.Title), Year: in.Year, PosterPath: in.PosterPath, ExternalIDs: ids}
			if err := s.resolve(ctx, &m); err != nil {
				return nil, err
			}
		}
		m.Local, m.Sources, m.Provenance = true, []string{}, []string{}
		if c.Members, err = coalesce(c.Members, m); err != nil {
			return nil, fmt.Errorf("%w: %q conflicts with an existing member's provider IDs", ErrConflict, m.Title)
		}
	}
	return c, s.store.Save(ctx, c, false)
}

func (s *Service) manageRoutes(r chi.Router) {
	r.Delete("/{id}", respond(func(_ http.ResponseWriter, r *http.Request) (any, error) {
		revision, err := strconv.Atoi(r.URL.Query().Get("revision"))
		if err != nil || revision < 0 {
			return nil, ErrInvalid
		}
		c, err := s.getForWrite(r.Context(), chi.URLParam(r, "id"))
		if err != nil {
			return nil, err
		}
		if c.Revision != revision {
			return nil, ErrConflict
		}
		suppressed, err := s.store.Delete(r.Context(), c.ID, revision)
		if err != nil {
			return nil, err
		}
		return map[string]any{"deleted": c.ID, "suppressed_sources": suppressed}, nil
	}))
	r.Post("/{id}/members", respond(func(w http.ResponseWriter, r *http.Request) (any, error) {
		var body struct {
			Items    []memberInput `json:"items"`
			Revision *int          `json:"revision,omitempty"`
		}
		if err := readBody(w, r, &body); err != nil {
			return nil, err
		}
		return s.AddMembers(r.Context(), chi.URLParam(r, "id"), body.Revision, body.Items)
	}))
	r.Delete("/{id}/members/{member}", respond(func(_ http.ResponseWriter, r *http.Request) (any, error) {
		c, err := s.getForWrite(r.Context(), chi.URLParam(r, "id"))
		if err != nil {
			return nil, err
		}
		for i, m := range c.Members {
			if m.ID != chi.URLParam(r, "member") {
				continue
			}
			if len(m.Sources) > 0 {
				if !m.Local {
					return nil, fmt.Errorf("%w: source titles can be excluded, not removed", ErrInvalid)
				}
				c.Members[i].Local = false
			} else {
				c.Members = append(c.Members[:i], c.Members[i+1:]...)
			}
			return c, s.store.Save(r.Context(), c, false)
		}
		return nil, sql.ErrNoRows
	}))
	r.Post("/{id}/bulk/add", respond(func(w http.ResponseWriter, r *http.Request) (any, error) {
		var body addBody
		if err := readBody(w, r, &body); err != nil {
			return nil, err
		}
		return s.PlanAdd(r.Context(), chi.URLParam(r, "id"), body)
	}))
	r.Post("/{id}/bulk/search", respond(func(w http.ResponseWriter, r *http.Request) (any, error) {
		var body searchBody
		if err := readBody(w, r, &body); err != nil {
			return nil, err
		}
		return s.PlanSearch(r.Context(), chi.URLParam(r, "id"), body)
	}))
	r.Post("/{id}/quality", respond(func(w http.ResponseWriter, r *http.Request) (any, error) {
		var body qualityBody
		if err := readBody(w, r, &body); err != nil {
			return nil, err
		}
		return s.PlanQuality(r.Context(), chi.URLParam(r, "id"), body)
	}))
	r.Get("/{id}/jobs", respond(func(_ http.ResponseWriter, r *http.Request) (any, error) {
		return map[string]any{"items": s.Jobs(chi.URLParam(r, "id"))}, nil
	}))
	r.Get("/{id}/jobs/{job}", respond(func(_ http.ResponseWriter, r *http.Request) (any, error) {
		return s.Job(chi.URLParam(r, "id"), chi.URLParam(r, "job"))
	}))
}

// validateProfile checks a collection default against existing profiles.
func (s *Service) validateProfile(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	actions, err := s.libraryActions()
	if err != nil {
		return err
	}
	profiles, err := actions.QualityProfiles(ctx)
	if err != nil {
		return err
	}
	if hasOption(profiles, id) == nil {
		return fmt.Errorf("%w: unknown quality profile", ErrInvalid)
	}
	return nil
}
