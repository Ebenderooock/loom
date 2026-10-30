package collections

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/ebenderooock/loom/internal/discover"
	"github.com/ebenderooock/loom/internal/importlists/providers"
	"github.com/ebenderooock/loom/internal/metadata/tmdb"
	"github.com/ebenderooock/loom/internal/metadata/tvdb"
	"github.com/ebenderooock/loom/internal/movies"
	"github.com/ebenderooock/loom/internal/series"
)

// SeriesLibrary supplies catalog and episode file state without acquisition.
type SeriesLibrary interface {
	discover.SeriesLibrary
	GetEpisodeStats(context.Context, string) (*series.EpisodeStats, error)
}

// Service reconciles persistent collections with explicit provider references.
type Service struct {
	store         *Store
	tmdb          *tmdb.Client
	tvdb          *tvdb.Client
	movies        discover.MovieLibrary
	series        SeriesLibrary
	tmdbListToken func() string
	// fetch is a test seam; production always uses the existing provider clients.
	fetch  func(context.Context, Source) (string, []Member, error)
	scanMu sync.Mutex

	jobsMu     sync.Mutex
	actions    Actions
	jobs       map[string]*Job
	jobOrder   []string
	activeKeys map[string]string
}

// NewService reuses the configured metadata clients and library services.
func NewService(store *Store, tmdbClient *tmdb.Client, tvdbClient *tvdb.Client, movies discover.MovieLibrary, shows SeriesLibrary, listToken func() string) *Service {
	s := &Service{store: store, tmdb: tmdbClient, tvdb: tvdbClient, movies: movies, series: shows, tmdbListToken: listToken}
	s.fetch = s.fetchSource
	return s
}

func putID(ids map[string]string, provider string, id *string) {
	if id != nil && *id != "" {
		ids[provider] = *id
	}
}

// resolve enriches exact IDs only. A failed mapping aborts refresh rather than
// silently replacing a title or using a name-search fallback.
func (s *Service) resolve(ctx context.Context, m *Member) error {
	if err := m.Validate(); err != nil {
		return err
	}
	original := make(map[string]string, len(m.ExternalIDs))
	for p, id := range m.ExternalIDs {
		original[p] = id
	}
	if m.ExternalIDs["tmdb"] == "" && s.tmdb.Configured() {
		provider := "imdb"
		id := m.ExternalIDs[provider]
		if id == "" && m.MediaType == "series" {
			provider, id = "tvdb", m.ExternalIDs["tvdb"]
		}
		if id != "" {
			n, err := s.tmdb.FindTitleID(ctx, m.MediaType, provider, id)
			if err != nil {
				return err
			}
			if n > 0 {
				m.ExternalIDs["tmdb"] = strconv.Itoa(n)
			}
		}
	}
	if id := m.ExternalIDs["tmdb"]; id != "" && s.tmdb.Configured() {
		if m.MediaType == "movie" {
			title, err := s.tmdb.GetMovie(ctx, number(id))
			if err != nil {
				return err
			}
			if title.TMDBID == nil || *title.TMDBID != id {
				return ErrConflict
			}
			putID(m.ExternalIDs, "imdb", title.IMDBID)
			putID(m.ExternalIDs, "tvdb", title.TVDBID)
			m.Title, m.Year, m.PosterPath, m.Overview, m.ReleaseDate = title.Title, title.Year, title.PosterPath, title.Overview, title.ReleaseDate
		} else {
			title, err := s.tmdb.GetTV(ctx, number(id))
			if err != nil {
				return err
			}
			if title.TMDBID == nil || *title.TMDBID != id {
				return ErrConflict
			}
			putID(m.ExternalIDs, "imdb", title.IMDBID)
			putID(m.ExternalIDs, "tvdb", title.TVDBID)
			m.Title, m.PosterPath, m.Overview, m.ReleaseDate = title.Title, title.PosterPath, title.Overview, title.FirstAirDate
			if len(title.FirstAirDate) >= 4 {
				m.Year = number(title.FirstAirDate[:4])
			}
		}
	}
	for p, id := range original {
		if m.ExternalIDs[p] != id {
			return ErrConflict
		}
	}
	m.ID = uuid.NewString()
	return m.Validate()
}

func (s *Service) fetchSource(ctx context.Context, ref Source) (string, []Member, error) {
	if err := ref.Validate(); err != nil {
		return "", nil, err
	}
	name := strings.ToUpper(ref.Provider) + " " + ref.Kind + " " + ref.ExternalID
	members := []Member{}
	switch ref.Provider {
	case "tmdb":
		if ref.Kind == "collection" {
			set, err := s.tmdb.GetCollection(ctx, number(ref.ExternalID))
			if err != nil {
				return "", nil, err
			}
			name = set.Name
			for _, part := range set.Parts {
				if part.Adult {
					continue
				}
				members = append(members, Member{MediaType: "movie", Title: part.Title, ExternalIDs: map[string]string{"tmdb": strconv.Itoa(part.ID)}})
			}
		} else {
			token := ""
			if s.tmdbListToken != nil {
				token = s.tmdbListToken()
			}
			items, err := providers.NewTMDbMixedList().Fetch(ctx, providers.ProviderConfig{URL: ref.ExternalID, APIKey: token})
			if err != nil {
				return "", nil, err
			}
			members = providerMembers(items)
		}
	case "imdb":
		items, err := providers.NewIMDbList().Fetch(ctx, providers.ProviderConfig{URL: ref.ExternalID})
		if err != nil {
			return "", nil, err
		}
		members = providerMembers(items)
	case "tvdb":
		list, err := s.tvdb.GetList(ctx, number(ref.ExternalID))
		if err != nil {
			return "", nil, err
		}
		name = list.Name
		sort.SliceStable(list.Entities, func(i, j int) bool { return list.Entities[i].Order < list.Entities[j].Order })
		for _, entity := range list.Entities {
			if entity.MovieID == 0 && entity.SeriesID == 0 {
				continue
			}
			if entity.MovieID > 0 && entity.SeriesID > 0 {
				return "", nil, fmt.Errorf("ambiguous TVDB list entity")
			}
			media, id := "movie", entity.MovieID
			if entity.SeriesID > 0 {
				media, id = "series", entity.SeriesID
			}
			title, err := s.tvdb.GetExtendedTitle(ctx, media, id)
			if err != nil {
				return "", nil, err
			}
			m := Member{MediaType: media, Title: title.Name, Year: number(title.Year), PosterPath: title.Image, Overview: title.Overview, ExternalIDs: map[string]string{"tvdb": strconv.Itoa(id)}}
			for _, remote := range title.RemoteIDs {
				switch strings.ToLower(remote.SourceName) {
				case "imdb":
					m.ExternalIDs["imdb"] = remote.ID
				case "themoviedb.com", "the movie database", "tmdb":
					m.ExternalIDs["tmdb"] = remote.ID
				}
			}
			members = append(members, m)
		}
	}
	if len(members) > 5000 {
		return "", nil, fmt.Errorf("source exceeds 5000 supported titles")
	}
	for i := range members {
		if err := s.resolve(ctx, &members[i]); err != nil {
			return "", nil, err
		}
	}
	return name, members, nil
}

func providerMembers(items []providers.Item) []Member {
	out := []Member{}
	for _, item := range items {
		if item.MediaType != "movie" && item.MediaType != "series" {
			continue
		}
		ids := map[string]string{}
		for p, id := range map[string]string{"imdb": item.IMDbID, "tmdb": item.TMDbID, "tvdb": item.TVDbID} {
			if id != "" {
				ids[p] = id
			}
		}
		out = append(out, Member{MediaType: item.MediaType, Title: item.Title, Year: item.Year, PosterPath: item.PosterPath, ExternalIDs: ids})
	}
	return out
}

func sourceError(ref Source) string {
	switch ref.Provider {
	case "imdb":
		return "IMDb list could not be read. Use a public ls… list; access may be restricted. Previous members are retained. Retry after checking source access."
	case "tvdb":
		return "TVDB list could not be read or mapped. Check TVDB API key/PIN, list access and title IDs. Previous members are retained."
	default:
		if ref.Kind == "list" {
			return "TMDB mixed list could not be read or mapped. Configure LOOM_TMDB_READ_TOKEN with a v4 read token and check list access. Previous members are retained."
		}
		return "TMDB collection could not be read or mapped. Check TMDB credentials and collection ID. Previous members are retained."
	}
}

// Refresh reconciles source contributions without discarding local intent.
func (s *Service) Refresh(ctx context.Context, c *Collection) error {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	for i, ref := range c.Sources {
		_, incoming, err := s.fetch(ctx, ref)
		if err != nil {
			c.Sources[i].Error = sourceError(ref)
			continue
		}
		// Work on a private copy: a mapping conflict must not partially apply a source.
		body, err := encode(c)
		if err != nil {
			return err
		}
		work, err := decode(string(body), c.Revision)
		if err != nil {
			return err
		}
		key := ref.Key()
		for j := range work.Members {
			filtered := []string{}
			for _, source := range work.Members[j].Sources {
				if source != key {
					filtered = append(filtered, source)
				}
			}
			work.Members[j].Sources = filtered
		}
		for _, m := range incoming {
			m.Sources = []string{key}
			work.Members, err = coalesce(work.Members, m)
			if err != nil {
				break
			}
		}
		if err != nil {
			c.Sources[i].Error = "External title IDs conflict. Previous members are retained; correct the source mapping before retrying."
			continue
		}
		active := []Member{}
		for _, m := range work.Members {
			if m.Local || m.Excluded || len(m.Sources) > 0 {
				active = append(active, m)
			}
		}
		c.Members = active
		c.Sources[i].Error = ""
		c.Sources[i].LastSync = time.Now().UTC().Format(time.RFC3339)
	}
	// Even a timeout records an explicit retryable failure, using the request's
	// parent context at the handler for persistence if this work budget expired.
	return nil
}

// Import creates a Loom collection from a new exact source identity. It never
// joins names/overlap; known source identities return their current owner.
// Import is an explicit user action, so it clears a deletion suppression.
func (s *Service) Import(ctx context.Context, ref Source) (*Collection, error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	if err := s.store.Unsuppress(ctx, ref); err != nil {
		return nil, err
	}
	return s.importSource(ctx, ref)
}

func (s *Service) importSource(ctx context.Context, ref Source) (*Collection, error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	if existing, err := s.store.FindSource(ctx, ref); err == nil {
		return existing, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	fetchCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	name, members, err := s.fetch(fetchCtx, ref)
	cancel()
	if err != nil {
		name, members, ref.Error = strings.ToUpper(ref.Provider)+" "+ref.ExternalID, []Member{}, sourceError(ref)
	} else {
		ref.LastSync = time.Now().UTC().Format(time.RFC3339)
	}
	c := &Collection{Name: name, Sources: []Source{ref}, Members: []Member{}}
	for _, m := range members {
		m.Sources = []string{ref.Key()}
		var err error
		c.Members, err = coalesce(c.Members, m)
		if err != nil {
			return nil, err
		}
	}
	if err := s.store.Save(ctx, c, true); err != nil {
		// A concurrent source import may have won its unique constraint.
		if existing, e := s.store.FindSource(ctx, ref); e == nil {
			return existing, nil
		}
		return nil, err
	}
	return c, nil
}

type libraryRecord struct {
	member  Member
	profile string
	file    func() (*bool, error)
	// missing counts monitored aired episodes without files (series only).
	missing func() (int, error)
}

func (s *Service) library(ctx context.Context) ([]libraryRecord, error) {
	out := []libraryRecord{}
	movieItems := []*movies.Movie{}
	for offset := 0; ; {
		page, err := s.movies.ListMovies(ctx, 1000, offset)
		if err != nil {
			return nil, err
		}
		movieItems = append(movieItems, page...)
		if len(page) < 1000 {
			break
		}
		offset += len(page)
	}
	for _, movie := range movieItems {
		if movie.DeletedAt != nil || movie.MonitoringStatus == "deleted" {
			continue
		}
		ids := map[string]string{}
		putID(ids, "tmdb", movie.TMDBID)
		putID(ids, "imdb", movie.IMDBID)
		putID(ids, "tvdb", movie.TVDBID)
		id := movie.ID
		out = append(out, libraryRecord{member: Member{MediaType: "movie", Title: movie.Title, Year: movie.Year, PosterPath: movie.PosterPath, Overview: movie.Overview, ReleaseDate: movie.ReleaseDate, ExternalIDs: ids, InLibrary: true, MediaID: id, LibraryID: movie.LibraryID, Monitored: movie.MonitoringStatus == "monitored"}, profile: movie.QualityProfileID, file: func() (*bool, error) {
			files, err := s.movies.ListMovieFiles(ctx, id)
			if err != nil {
				return nil, err
			}
			has := false
			for _, f := range files {
				if f != nil && f.DeletedAt == nil {
					has = true
				}
			}
			return &has, nil
		}})
	}
	shows, err := s.series.ListSeries(ctx)
	if err != nil {
		return nil, err
	}
	for _, show := range shows {
		ids := map[string]string{}
		putID(ids, "tmdb", show.TMDBID)
		putID(ids, "imdb", show.IMDBID)
		putID(ids, "tvdb", show.TVDBID)
		id := show.ID
		out = append(out, libraryRecord{member: Member{MediaType: "series", Title: show.Title, Year: show.Year, PosterPath: show.PosterPath, Overview: show.Overview, ExternalIDs: ids, InLibrary: true, MediaID: id, LibraryID: show.LibraryID, Monitored: show.MonitoringStatus != "none" && show.MonitoringStatus != "unmonitored" && show.MonitoringStatus != "archived"}, profile: show.QualityProfileID, file: func() (*bool, error) {
			stats, err := s.series.GetEpisodeStats(ctx, id)
			if err != nil {
				return nil, err
			}
			has := stats != nil && stats.DownloadedEpisodes > 0
			return &has, nil
		}, missing: func() (int, error) {
			stats, err := s.series.GetEpisodeStats(ctx, id)
			if err != nil || stats == nil {
				return 0, err
			}
			return stats.MissingEpisodes, nil
		}})
	}
	return out, nil
}

func decorate(c *Collection, library []libraryRecord, files bool) error {
	c.Total, c.InLibrary = 0, 0
	for i := range c.Members {
		m := &c.Members[i]
		m.InLibrary, m.MediaID, m.LibraryID, m.Monitored, m.HasFile = false, "", "", false, nil
		for _, record := range library {
			if record.member.MediaType != m.MediaType {
				continue
			}
			match, conflict := false, false
			for p, id := range m.ExternalIDs {
				if other := record.member.ExternalIDs[p]; other != "" {
					if other == id {
						match = true
					} else {
						conflict = true
					}
				}
			}
			if !match || conflict {
				continue
			}
			if m.InLibrary && m.MediaID != record.member.MediaID {
				return ErrConflict
			}
			m.InLibrary, m.MediaID, m.LibraryID, m.Monitored = true, record.member.MediaID, record.member.LibraryID, record.member.Monitored
			if files {
				has, err := record.file()
				if err != nil {
					return err
				}
				m.HasFile = has
			}
		}
		if !m.InLibrary {
			no := false
			m.HasFile = &no
		}
		if !m.Excluded {
			c.Total++
			if m.InLibrary {
				c.InLibrary++
			}
		}
	}
	return nil
}

// Discover imports exact provider-declared sets/lists. Production discovery runs
// through ScanBatch so progress survives interruption and request lifetimes.
func (s *Service) Discover(ctx context.Context) ([]*Collection, []string, error) {
	library, err := s.library(ctx)
	if err != nil {
		return nil, nil, err
	}
	out, warnings := []*Collection{}, []string{}
	for _, record := range library {
		items, err := s.discoverMember(ctx, record.member)
		if err != nil {
			warnings = append(warnings, err.Error())
		}
		out = append(out, items...)
	}
	return out, warnings, nil
}

func (s *Service) discoverMember(ctx context.Context, m Member) ([]*Collection, error) {
	refs := []Source{}
	var failures []error
	if m.ExternalIDs["tmdb"] == "" && m.ExternalIDs["imdb"] != "" {
		id, err := s.tmdb.FindTitleID(ctx, m.MediaType, "imdb", m.ExternalIDs["imdb"])
		if err != nil {
			failures = append(failures, errors.New("IMDb identity mapping failed; check metadata credentials and retry"))
		} else if id > 0 {
			m.ExternalIDs["tmdb"] = strconv.Itoa(id)
		}
	}
	if m.MediaType == "series" && m.ExternalIDs["tvdb"] == "" && m.ExternalIDs["tmdb"] != "" {
		title, err := s.tmdb.GetTV(ctx, number(m.ExternalIDs["tmdb"]))
		if err != nil {
			failures = append(failures, errors.New("TVDB identity mapping failed; check metadata credentials and retry"))
		} else {
			putID(m.ExternalIDs, "tvdb", title.TVDBID)
		}
	}
	if m.MediaType == "movie" && m.ExternalIDs["tmdb"] != "" {
		title, err := s.tmdb.GetMovieDetails(ctx, number(m.ExternalIDs["tmdb"]))
		if err != nil {
			failures = append(failures, errors.New("TMDB candidate discovery failed; check metadata credentials and retry"))
		} else if title.Collection != nil {
			refs = append(refs, Source{Provider: "tmdb", Kind: "collection", ExternalID: strconv.Itoa(title.Collection.ID)})
		}
	}
	if m.ExternalIDs["tvdb"] != "" {
		title, err := s.tvdb.GetExtendedTitle(ctx, m.MediaType, number(m.ExternalIDs["tvdb"]))
		if err != nil {
			failures = append(failures, errors.New("TVDB candidate discovery failed; check metadata credentials and retry"))
		} else {
			for _, list := range title.Lists {
				if list.IsOfficial && list.ID > 0 {
					refs = append(refs, Source{Provider: "tvdb", Kind: "list", ExternalID: strconv.Itoa(list.ID)})
				}
			}
		}
	}
	if m.ExternalIDs["tvdb"] == "" && (m.MediaType != "movie" || m.ExternalIDs["tmdb"] == "") {
		failures = append(failures, errors.New("no provider-declared collection lookup for these IDs; import a known source directly"))
	}
	out := []*Collection{}
	for _, ref := range refs {
		// Deleted collections stay deleted until the user imports the source.
		if suppressed, err := s.store.Suppressed(ctx, ref); err != nil {
			return out, err
		} else if suppressed {
			continue
		}
		existing, lookupErr := s.store.FindSource(ctx, ref)
		if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
			return out, lookupErr
		}
		if existing != nil {
			failed := false
			for _, source := range existing.Sources {
				failed = failed || source.Error != ""
			}
			if failed {
				if err := s.Refresh(ctx, existing); err != nil {
					return out, err
				}
				if err := s.store.Save(ctx, existing, false); err != nil {
					return out, err
				}
			}
		}
		c, err := s.importSource(ctx, ref)
		if err != nil {
			return out, err
		}
		for _, source := range c.Sources {
			if source.Error != "" {
				failures = append(failures, errors.New(source.Error))
			}
		}
		out = append(out, c)
	}
	return out, errors.Join(failures...)
}
