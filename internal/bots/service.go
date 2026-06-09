package bots

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"

	"github.com/ebenderooock/loom/internal/requests"
)

// MediaResult is a platform-agnostic search/lookup result.
type MediaResult struct {
	MediaType  requests.MediaType
	TMDBID     string
	Title      string
	Year       int
	Overview   string
	PosterPath string
}

// RequestService is the subset of the media-requests service the bots need.
type RequestService interface {
	Create(ctx context.Context, userID, username string, isAdmin bool, in requests.CreateInput) (requests.Request, error)
	ListMine(ctx context.Context, userID string) ([]requests.Request, error)
	ListAll(ctx context.Context, status requests.Status) ([]requests.Request, error)
	Get(ctx context.Context, id string) (requests.Request, error)
	Approve(ctx context.Context, id, qualityProfileID, libraryID, decidedBy string) (requests.Request, error)
	Reject(ctx context.Context, id, reason, decidedBy string) (requests.Request, error)
}

// SearchService resolves catalog searches and trusted by-id lookups. By-id
// lookups are used to re-fetch metadata server-side when a request is submitted,
// so caller-supplied button payloads are never trusted for fulfillment data.
type SearchService interface {
	SearchMovies(ctx context.Context, query string) ([]MediaResult, error)
	SearchSeries(ctx context.Context, query string) ([]MediaResult, error)
	SearchArtists(ctx context.Context, query string) ([]MediaResult, error)
	GetMovie(ctx context.Context, tmdbID string) (*MediaResult, error)
	GetSeries(ctx context.Context, tmdbID string) (*MediaResult, error)
	GetArtist(ctx context.Context, mbid string) (*MediaResult, error)
}

// UserDirectory resolves a Loom user's display name and admin status.
type UserDirectory interface {
	Lookup(ctx context.Context, userID int64) (username string, isAdmin bool, err error)
}

// Button is a tappable action rendered as an inline keyboard button (Telegram)
// or a message component (Discord). Data is an opaque callback payload.
type Button struct {
	Label string
	Data  string
}

// Reply is a platform-agnostic bot response. Transports render Text and any
// Buttons; Discord responses are sent ephemerally to avoid leaking in channels.
type Reply struct {
	Text    string
	Buttons []Button
}

// Command is a single inbound interaction from a transport: either a text
// command (Text set) or a button press (Callback set).
type Command struct {
	Platform         Platform
	ExternalID       string
	ExternalUsername string
	Text             string
	Callback         string
}

// Service is the platform-agnostic bot brain.
type Service struct {
	store    *Store
	requests RequestService
	search   SearchService
	users    UserDirectory
	limiter  *rateLimiter
	logger   *slog.Logger

	maxResults int
}

// Options configures a bot Service.
type Options struct {
	Store    *Store
	Requests RequestService
	Search   SearchService
	Users    UserDirectory
	Logger   *slog.Logger
}

// NewService constructs a bot brain.
func NewService(opts Options) *Service {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		store:      opts.Store,
		requests:   opts.Requests,
		search:     opts.Search,
		users:      opts.Users,
		limiter:    newRateLimiter(15, 20), // 15 burst, 20/min sustained per identity
		logger:     logger,
		maxResults: 6,
	}
}

// Store exposes the underlying store (for the supervisor/config reload).
func (s *Service) Store() *Store { return s.store }

const linkPrompt = "🔗 Link your Loom account first.\nSend /link to get a code, then enter it in Loom under Settings → Request Bots."

const helpText = "🎬 *Loom request bot*\n\n" +
	"/search <title> — find a movie or show to request\n" +
	"/status — see your requests\n" +
	"/link — link this chat to your Loom account\n" +
	"/help — show this message\n\n" +
	"Admins: /pending — review awaiting requests"

// Handle processes one inbound command and returns a reply to render. It never
// returns an error; failures are converted to a user-facing message.
func (s *Service) Handle(ctx context.Context, cmd Command) Reply {
	key := string(cmd.Platform) + ":" + cmd.ExternalID
	if !s.limiter.allow(key) {
		return Reply{Text: "⏳ You're going a bit fast — try again in a few seconds."}
	}
	if cmd.Callback != "" {
		return s.handleCallback(ctx, cmd)
	}
	return s.handleText(ctx, cmd)
}

func (s *Service) handleText(ctx context.Context, cmd Command) Reply {
	name, args := parseCommand(cmd.Text)
	switch name {
	case "start", "help", "":
		return Reply{Text: helpText}
	case "link":
		return s.cmdLink(ctx, cmd)
	case "search":
		return s.cmdSearch(ctx, cmd, args)
	case "status":
		return s.cmdStatus(ctx, cmd)
	case "pending":
		return s.cmdPending(ctx, cmd)
	default:
		return Reply{Text: "Unknown command. " + helpText}
	}
}

// parseCommand splits "/search the matrix" into ("search", "the matrix").
func parseCommand(text string) (name, args string) {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "/")
	if text == "" {
		return "", ""
	}
	parts := strings.SplitN(text, " ", 2)
	name = strings.ToLower(parts[0])
	// Tolerate Telegram's "/search@MyBot" mention suffix.
	if i := strings.IndexByte(name, '@'); i >= 0 {
		name = name[:i]
	}
	if len(parts) == 2 {
		args = strings.TrimSpace(parts[1])
	}
	return name, args
}

func (s *Service) cmdLink(ctx context.Context, cmd Command) Reply {
	// If already linked, say so rather than minting a code.
	if link, err := s.store.GetLink(ctx, cmd.Platform, cmd.ExternalID); err == nil && link != nil {
		if name, _, err := s.users.Lookup(ctx, link.UserID); err == nil {
			return Reply{Text: "✓ This chat is already linked to Loom account *" + name + "*.\nUnlink it in Settings → Request Bots to re-link."}
		}
	}
	lc, err := s.store.CreateLinkCode(ctx, cmd.Platform, cmd.ExternalID, cmd.ExternalUsername)
	if err != nil {
		s.logger.Error("bots: create link code", "err", err)
		return Reply{Text: "Sorry, I couldn't create a link code. Try again later."}
	}
	return Reply{Text: "🔗 Your link code is:\n\n*" + lc.Code + "*\n\n" +
		"In Loom, go to Settings → Request Bots → Link account and enter this code. It expires in 10 minutes."}
}

func (s *Service) cmdSearch(ctx context.Context, cmd Command, query string) Reply {
	caller, ok := s.requireLinked(ctx, cmd)
	if !ok {
		return Reply{Text: linkPrompt}
	}
	_ = caller
	query = strings.TrimSpace(query)
	if query == "" {
		return Reply{Text: "Usage: /search <title>"}
	}
	movies, err := s.search.SearchMovies(ctx, query)
	if err != nil {
		s.logger.Warn("bots: movie search", "err", err)
	}
	series, err := s.search.SearchSeries(ctx, query)
	if err != nil {
		s.logger.Warn("bots: series search", "err", err)
	}
	artists, err := s.search.SearchArtists(ctx, query)
	if err != nil {
		s.logger.Warn("bots: artist search", "err", err)
	}
	results := interleave(movies, series, artists, s.maxResults)
	if len(results) == 0 {
		return Reply{Text: "No results for “" + query + "”."}
	}
	reply := Reply{Text: "Results for “" + query + "” — tap to request:"}
	for _, r := range results {
		reply.Buttons = append(reply.Buttons, Button{
			Label: resultLabel(r),
			Data:  encodeRequest(r.MediaType, r.TMDBID),
		})
	}
	return reply
}

func (s *Service) cmdStatus(ctx context.Context, cmd Command) Reply {
	caller, ok := s.requireLinked(ctx, cmd)
	if !ok {
		return Reply{Text: linkPrompt}
	}
	list, err := s.requests.ListMine(ctx, caller.userID)
	if err != nil {
		s.logger.Error("bots: list mine", "err", err)
		return Reply{Text: "Sorry, I couldn't load your requests."}
	}
	if len(list) == 0 {
		return Reply{Text: "You have no requests yet. Use /search to add one."}
	}
	var b strings.Builder
	b.WriteString("📋 *Your requests*\n")
	for _, r := range list {
		b.WriteString("\n" + statusEmoji(r.Status) + " " + r.Title)
		if r.Year > 0 {
			b.WriteString(" (" + strconv.Itoa(r.Year) + ")")
		}
		b.WriteString(" — " + string(r.Status))
	}
	return Reply{Text: b.String()}
}

func (s *Service) cmdPending(ctx context.Context, cmd Command) Reply {
	caller, ok := s.requireLinked(ctx, cmd)
	if !ok {
		return Reply{Text: linkPrompt}
	}
	if !caller.isAdmin {
		return Reply{Text: "⛔ Only admins can review pending requests."}
	}
	list, err := s.requests.ListAll(ctx, requests.StatusPending)
	if err != nil {
		s.logger.Error("bots: list pending", "err", err)
		return Reply{Text: "Sorry, I couldn't load pending requests."}
	}
	if len(list) == 0 {
		return Reply{Text: "✓ No pending requests."}
	}
	// Render the oldest pending request with action buttons. Admins call
	// /pending repeatedly to work through the queue.
	r := list[0]
	text := "⏳ *Pending* (" + strconv.Itoa(len(list)) + " total)\n\n" +
		r.Title
	if r.Year > 0 {
		text += " (" + strconv.Itoa(r.Year) + ")"
	}
	text += "\nRequested by " + r.Username + " · " + string(r.MediaType)
	if r.Overview != "" {
		text += "\n\n" + truncate(r.Overview, 240)
	}
	return Reply{
		Text: text,
		Buttons: []Button{
			{Label: "✅ Approve", Data: "apr|" + r.ID},
			{Label: "❌ Reject", Data: "rej|" + r.ID},
		},
	}
}

func (s *Service) handleCallback(ctx context.Context, cmd Command) Reply {
	action, a1, a2 := decodeCallback(cmd.Callback)
	switch action {
	case "req":
		return s.callbackRequest(ctx, cmd, requests.MediaType(a1), a2)
	case "apr":
		return s.callbackDecision(ctx, cmd, a1, true)
	case "rej":
		return s.callbackDecision(ctx, cmd, a1, false)
	default:
		return Reply{Text: "That action is no longer available."}
	}
}

func (s *Service) callbackRequest(ctx context.Context, cmd Command, mt requests.MediaType, tmdbID string) Reply {
	caller, ok := s.requireLinked(ctx, cmd)
	if !ok {
		return Reply{Text: linkPrompt}
	}
	// Trusted server-side re-fetch: never trust the button's media fields.
	var (
		res *MediaResult
		err error
	)
	switch mt {
	case requests.MediaMovie:
		res, err = s.search.GetMovie(ctx, tmdbID)
	case requests.MediaSeries:
		res, err = s.search.GetSeries(ctx, tmdbID)
	case requests.MediaArtist:
		res, err = s.search.GetArtist(ctx, tmdbID)
	default:
		return Reply{Text: "Unsupported media type."}
	}
	if err != nil || res == nil {
		s.logger.Warn("bots: refetch metadata", "tmdb", tmdbID, "err", err)
		return Reply{Text: "Sorry, I couldn't look that title up. Try /search again."}
	}

	in := requests.CreateInput{
		MediaType:  mt,
		TMDBID:     res.TMDBID,
		Title:      res.Title,
		Year:       res.Year,
		PosterPath: res.PosterPath,
		Overview:   res.Overview,
	}
	_, err = s.requests.Create(ctx, caller.userID, caller.username, caller.isAdmin, in)
	switch {
	case err == nil:
		return Reply{Text: "✅ Request submitted for *" + res.Title + "*. You'll be notified when it's ready."}
	case errors.Is(err, requests.ErrAlreadyAvailable):
		return Reply{Text: "✓ *" + res.Title + "* is already available in the library."}
	case errors.Is(err, requests.ErrQuotaExceeded):
		return Reply{Text: "⚠️ You've reached your request quota. Please try again later."}
	case errors.Is(err, requests.ErrDuplicate):
		return Reply{Text: "↻ You already have an open request for *" + res.Title + "*."}
	default:
		s.logger.Error("bots: create request", "err", err)
		return Reply{Text: "Sorry, something went wrong submitting that request."}
	}
}

func (s *Service) callbackDecision(ctx context.Context, cmd Command, reqID string, approve bool) Reply {
	caller, ok := s.requireLinked(ctx, cmd)
	if !ok {
		return Reply{Text: linkPrompt}
	}
	if !caller.isAdmin {
		return Reply{Text: "⛔ Only admins can approve or reject requests."}
	}

	// Idempotency: surface a friendly message if it was already decided.
	existing, err := s.requests.Get(ctx, reqID)
	if err != nil {
		return Reply{Text: "That request no longer exists."}
	}
	if existing.Status != requests.StatusPending && existing.Status != requests.StatusFailed {
		return Reply{Text: "ℹ️ *" + existing.Title + "* was already " + string(existing.Status) + "."}
	}

	if !approve {
		if _, err := s.requests.Reject(ctx, reqID, "rejected via chat", caller.username); err != nil {
			s.logger.Error("bots: reject", "err", err)
			return Reply{Text: "Sorry, I couldn't reject that request."}
		}
		return Reply{Text: "❌ Rejected *" + existing.Title + "*."}
	}

	qp, lib, missing := s.approvalTarget(ctx, existing.MediaType)
	if missing != "" {
		return Reply{Text: "⚠️ Can't approve from chat: no default " + missing + " is configured.\n" +
			"Set chat-approval defaults in Settings → Request Bots, or approve in the web UI."}
	}
	if _, err := s.requests.Approve(ctx, reqID, qp, lib, caller.username); err != nil {
		s.logger.Error("bots: approve", "err", err)
		return Reply{Text: "Sorry, I couldn't approve that request: " + err.Error()}
	}
	return Reply{Text: "✅ Approved *" + existing.Title + "* — searching now."}
}

// approvalTarget returns the configured default quality profile + library for a
// media type, or a description of the first missing default.
func (s *Service) approvalTarget(ctx context.Context, mt requests.MediaType) (qp, lib, missing string) {
	cfg, err := s.store.GetConfig(ctx)
	if err != nil {
		return "", "", "configuration"
	}
	if mt == requests.MediaMovie {
		if cfg.DefaultMovieQualityProfileID == "" {
			return "", "", "movie quality profile"
		}
		if cfg.DefaultMovieLibraryID == "" {
			return "", "", "movie library"
		}
		return cfg.DefaultMovieQualityProfileID, cfg.DefaultMovieLibraryID, ""
	}
	if mt == requests.MediaArtist {
		if cfg.DefaultMusicQualityProfileID == "" {
			return "", "", "music quality profile"
		}
		if cfg.DefaultMusicLibraryID == "" {
			return "", "", "music library"
		}
		return cfg.DefaultMusicQualityProfileID, cfg.DefaultMusicLibraryID, ""
	}
	if cfg.DefaultSeriesQualityProfileID == "" {
		return "", "", "series quality profile"
	}
	if cfg.DefaultSeriesLibraryID == "" {
		return "", "", "series library"
	}
	return cfg.DefaultSeriesQualityProfileID, cfg.DefaultSeriesLibraryID, ""
}

// caller carries the resolved Loom identity for a chat user.
type caller struct {
	userID   string
	username string
	isAdmin  bool
}

// requireLinked resolves the chat identity to a Loom user, returning ok=false
// when the chat is not yet linked or the user can't be resolved.
func (s *Service) requireLinked(ctx context.Context, cmd Command) (caller, bool) {
	link, err := s.store.GetLink(ctx, cmd.Platform, cmd.ExternalID)
	if err != nil {
		s.logger.Error("bots: get link", "err", err)
		return caller{}, false
	}
	if link == nil {
		return caller{}, false
	}
	name, isAdmin, err := s.users.Lookup(ctx, link.UserID)
	if err != nil {
		s.logger.Warn("bots: resolve user", "user_id", link.UserID, "err", err)
		return caller{}, false
	}
	return caller{
		userID:   strconv.FormatInt(link.UserID, 10),
		username: name,
		isAdmin:  isAdmin,
	}, true
}
