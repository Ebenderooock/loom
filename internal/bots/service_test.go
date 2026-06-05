package bots

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/ebenderooock/loom/internal/requests"
)

// --- fakes ---------------------------------------------------------------

type fakeRequests struct {
	created   []requests.CreateInput
	mine      []requests.Request
	pending   []requests.Request
	byID      map[string]requests.Request
	createErr error
	approved  []string
	rejected  []string
}

func (f *fakeRequests) Create(_ context.Context, _, _ string, _ bool, in requests.CreateInput) (requests.Request, error) {
	if f.createErr != nil {
		return requests.Request{}, f.createErr
	}
	f.created = append(f.created, in)
	return requests.Request{ID: "new", Title: in.Title, Status: requests.StatusPending}, nil
}
func (f *fakeRequests) ListMine(context.Context, string) ([]requests.Request, error) {
	return f.mine, nil
}
func (f *fakeRequests) ListAll(_ context.Context, st requests.Status) ([]requests.Request, error) {
	return f.pending, nil
}
func (f *fakeRequests) Get(_ context.Context, id string) (requests.Request, error) {
	r, ok := f.byID[id]
	if !ok {
		return requests.Request{}, sql.ErrNoRows
	}
	return r, nil
}
func (f *fakeRequests) Approve(_ context.Context, id, qp, lib, by string) (requests.Request, error) {
	f.approved = append(f.approved, id+"|"+qp+"|"+lib)
	return requests.Request{ID: id, Status: requests.StatusApproved}, nil
}
func (f *fakeRequests) Reject(_ context.Context, id, reason, by string) (requests.Request, error) {
	f.rejected = append(f.rejected, id)
	return requests.Request{ID: id, Status: requests.StatusRejected}, nil
}

type fakeSearch struct {
	movies []MediaResult
	series []MediaResult
}

func (f *fakeSearch) SearchMovies(context.Context, string) ([]MediaResult, error) {
	return f.movies, nil
}
func (f *fakeSearch) SearchSeries(context.Context, string) ([]MediaResult, error) {
	return f.series, nil
}
func (f *fakeSearch) GetMovie(_ context.Context, tmdb string) (*MediaResult, error) {
	return &MediaResult{MediaType: requests.MediaMovie, TMDBID: tmdb, Title: "Movie " + tmdb, Year: 2020}, nil
}
func (f *fakeSearch) GetSeries(_ context.Context, tmdb string) (*MediaResult, error) {
	return &MediaResult{MediaType: requests.MediaSeries, TMDBID: tmdb, Title: "Series " + tmdb}, nil
}

type fakeUsers struct {
	name    string
	isAdmin bool
	err     error
}

func (f *fakeUsers) Lookup(context.Context, int64) (string, bool, error) {
	return f.name, f.isAdmin, f.err
}

// --- harness -------------------------------------------------------------

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	schema := `
	CREATE TABLE bot_config (
		id INTEGER PRIMARY KEY CHECK (id=1),
		telegram_enabled BOOLEAN NOT NULL DEFAULT 0,
		telegram_bot_token TEXT NOT NULL DEFAULT '',
		discord_enabled BOOLEAN NOT NULL DEFAULT 0,
		discord_bot_token TEXT NOT NULL DEFAULT '',
		default_movie_quality_profile_id TEXT NOT NULL DEFAULT '',
		default_movie_library_id TEXT NOT NULL DEFAULT '',
		default_series_quality_profile_id TEXT NOT NULL DEFAULT '',
		default_series_library_id TEXT NOT NULL DEFAULT '',
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	INSERT INTO bot_config (id) VALUES (1);
	CREATE TABLE bot_account_links (
		id TEXT PRIMARY KEY, platform TEXT NOT NULL, external_id TEXT NOT NULL,
		external_username TEXT NOT NULL DEFAULT '', user_id INTEGER NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(platform, external_id)
	);
	CREATE TABLE bot_link_codes (
		code TEXT PRIMARY KEY, platform TEXT NOT NULL, external_id TEXT NOT NULL,
		external_username TEXT NOT NULL DEFAULT '', expires_at TIMESTAMP NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	return NewStore(db)
}

func newTestService(t *testing.T, st *Store, r RequestService, sr SearchService, u UserDirectory) *Service {
	return NewService(Options{Store: st, Requests: r, Search: sr, Users: u})
}

func link(t *testing.T, st *Store, p Platform, ext string, userID int64) {
	t.Helper()
	lc, err := st.CreateLinkCode(context.Background(), p, ext, "handle")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.RedeemLinkCode(context.Background(), lc.Code, userID); err != nil {
		t.Fatal(err)
	}
}

// --- tests ---------------------------------------------------------------

func TestHandle_UnlinkedGatedCommandPromptsLink(t *testing.T) {
	st := newTestStore(t)
	svc := newTestService(t, st, &fakeRequests{}, &fakeSearch{}, &fakeUsers{})
	r := svc.Handle(context.Background(), Command{Platform: PlatformTelegram, ExternalID: "u1", Text: "/search matrix"})
	if !strings.Contains(r.Text, "Link your Loom account") {
		t.Fatalf("expected link prompt, got %q", r.Text)
	}
}

func TestHandle_HelpNoLinkRequired(t *testing.T) {
	st := newTestStore(t)
	svc := newTestService(t, st, &fakeRequests{}, &fakeSearch{}, &fakeUsers{})
	r := svc.Handle(context.Background(), Command{Platform: PlatformTelegram, ExternalID: "u1", Text: "/help"})
	if !strings.Contains(r.Text, "request bot") {
		t.Fatalf("expected help, got %q", r.Text)
	}
}

func TestHandle_LinkGeneratesCode(t *testing.T) {
	st := newTestStore(t)
	svc := newTestService(t, st, &fakeRequests{}, &fakeSearch{}, &fakeUsers{})
	r := svc.Handle(context.Background(), Command{Platform: PlatformTelegram, ExternalID: "u1", ExternalUsername: "alice", Text: "/link"})
	if !strings.Contains(r.Text, "link code") {
		t.Fatalf("expected code message, got %q", r.Text)
	}
}

func TestHandle_SearchRendersRequestButtons(t *testing.T) {
	st := newTestStore(t)
	link(t, st, PlatformTelegram, "u1", 7)
	sr := &fakeSearch{
		movies: []MediaResult{{MediaType: requests.MediaMovie, TMDBID: "11", Title: "The Matrix", Year: 1999}},
		series: []MediaResult{{MediaType: requests.MediaSeries, TMDBID: "22", Title: "Matrix Show"}},
	}
	svc := newTestService(t, st, &fakeRequests{}, sr, &fakeUsers{name: "alice"})
	r := svc.Handle(context.Background(), Command{Platform: PlatformTelegram, ExternalID: "u1", Text: "/search matrix"})
	if len(r.Buttons) != 2 {
		t.Fatalf("expected 2 buttons, got %d", len(r.Buttons))
	}
	if r.Buttons[0].Data != "req|movie|11" {
		t.Fatalf("unexpected button data %q", r.Buttons[0].Data)
	}
}

func TestHandle_RequestButtonRefetchesAndCreates(t *testing.T) {
	st := newTestStore(t)
	link(t, st, PlatformTelegram, "u1", 7)
	fr := &fakeRequests{}
	svc := newTestService(t, st, fr, &fakeSearch{}, &fakeUsers{name: "alice"})
	r := svc.Handle(context.Background(), Command{Platform: PlatformTelegram, ExternalID: "u1", Callback: "req|movie|11"})
	if len(fr.created) != 1 {
		t.Fatalf("expected 1 created request, got %d", len(fr.created))
	}
	// Trusted re-fetch: title comes from the search service, not the payload.
	if fr.created[0].Title != "Movie 11" || fr.created[0].TMDBID != "11" {
		t.Fatalf("unexpected create input %+v", fr.created[0])
	}
	if !strings.Contains(r.Text, "Request submitted") {
		t.Fatalf("unexpected reply %q", r.Text)
	}
}

func TestHandle_RequestQuotaExceeded(t *testing.T) {
	st := newTestStore(t)
	link(t, st, PlatformTelegram, "u1", 7)
	fr := &fakeRequests{createErr: requests.ErrQuotaExceeded}
	svc := newTestService(t, st, fr, &fakeSearch{}, &fakeUsers{name: "alice"})
	r := svc.Handle(context.Background(), Command{Platform: PlatformTelegram, ExternalID: "u1", Callback: "req|movie|11"})
	if !strings.Contains(r.Text, "quota") {
		t.Fatalf("expected quota message, got %q", r.Text)
	}
}

func TestHandle_RequestAlreadyAvailable(t *testing.T) {
	st := newTestStore(t)
	link(t, st, PlatformTelegram, "u1", 7)
	fr := &fakeRequests{createErr: requests.ErrAlreadyAvailable}
	svc := newTestService(t, st, fr, &fakeSearch{}, &fakeUsers{name: "alice"})
	r := svc.Handle(context.Background(), Command{Platform: PlatformTelegram, ExternalID: "u1", Callback: "req|series|22"})
	if !strings.Contains(r.Text, "already available") {
		t.Fatalf("expected already-available message, got %q", r.Text)
	}
}

func TestHandle_PendingAdminOnly(t *testing.T) {
	st := newTestStore(t)
	link(t, st, PlatformTelegram, "u1", 7)
	fr := &fakeRequests{pending: []requests.Request{{ID: "r1", Title: "Dune", Year: 2021, Username: "bob", MediaType: requests.MediaMovie}}}
	// non-admin
	svc := newTestService(t, st, fr, &fakeSearch{}, &fakeUsers{name: "alice", isAdmin: false})
	r := svc.Handle(context.Background(), Command{Platform: PlatformTelegram, ExternalID: "u1", Text: "/pending"})
	if !strings.Contains(r.Text, "Only admins") {
		t.Fatalf("expected admin-only message, got %q", r.Text)
	}
	// admin
	svc = newTestService(t, st, fr, &fakeSearch{}, &fakeUsers{name: "alice", isAdmin: true})
	r = svc.Handle(context.Background(), Command{Platform: PlatformTelegram, ExternalID: "u1", Text: "/pending"})
	if len(r.Buttons) != 2 || r.Buttons[0].Data != "apr|r1" {
		t.Fatalf("expected approve/reject buttons, got %+v", r.Buttons)
	}
}

func TestHandle_ApproveRequiresDefaults(t *testing.T) {
	st := newTestStore(t)
	link(t, st, PlatformTelegram, "u1", 7)
	fr := &fakeRequests{byID: map[string]requests.Request{
		"r1": {ID: "r1", Title: "Dune", MediaType: requests.MediaMovie, Status: requests.StatusPending},
	}}
	svc := newTestService(t, st, fr, &fakeSearch{}, &fakeUsers{name: "admin", isAdmin: true})
	// No defaults configured yet → graceful failure.
	r := svc.Handle(context.Background(), Command{Platform: PlatformTelegram, ExternalID: "u1", Callback: "apr|r1"})
	if !strings.Contains(r.Text, "no default") || len(fr.approved) != 0 {
		t.Fatalf("expected missing-default message, got %q approved=%v", r.Text, fr.approved)
	}
	// Configure defaults → approval proceeds.
	cfg, _ := st.GetConfig(context.Background())
	cfg.DefaultMovieQualityProfileID = "qp1"
	cfg.DefaultMovieLibraryID = "lib1"
	if err := st.SetConfig(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	r = svc.Handle(context.Background(), Command{Platform: PlatformTelegram, ExternalID: "u1", Callback: "apr|r1"})
	if len(fr.approved) != 1 || fr.approved[0] != "r1|qp1|lib1" {
		t.Fatalf("expected approval with defaults, got %v (%q)", fr.approved, r.Text)
	}
}

func TestHandle_ApproveNonAdminDenied(t *testing.T) {
	st := newTestStore(t)
	link(t, st, PlatformTelegram, "u1", 7)
	fr := &fakeRequests{byID: map[string]requests.Request{"r1": {ID: "r1", Status: requests.StatusPending}}}
	svc := newTestService(t, st, fr, &fakeSearch{}, &fakeUsers{name: "alice", isAdmin: false})
	r := svc.Handle(context.Background(), Command{Platform: PlatformTelegram, ExternalID: "u1", Callback: "apr|r1"})
	if !strings.Contains(r.Text, "Only admins") || len(fr.approved) != 0 {
		t.Fatalf("expected denial, got %q", r.Text)
	}
}

func TestHandle_DecisionIdempotentWhenAlreadyDecided(t *testing.T) {
	st := newTestStore(t)
	link(t, st, PlatformTelegram, "u1", 7)
	fr := &fakeRequests{byID: map[string]requests.Request{
		"r1": {ID: "r1", Title: "Dune", Status: requests.StatusApproved},
	}}
	svc := newTestService(t, st, fr, &fakeSearch{}, &fakeUsers{name: "admin", isAdmin: true})
	r := svc.Handle(context.Background(), Command{Platform: PlatformTelegram, ExternalID: "u1", Callback: "apr|r1"})
	if !strings.Contains(r.Text, "already approved") || len(fr.approved) != 0 {
		t.Fatalf("expected idempotent message, got %q", r.Text)
	}
}

func TestHandle_RejectDecision(t *testing.T) {
	st := newTestStore(t)
	link(t, st, PlatformTelegram, "u1", 7)
	fr := &fakeRequests{byID: map[string]requests.Request{
		"r1": {ID: "r1", Title: "Dune", Status: requests.StatusPending},
	}}
	svc := newTestService(t, st, fr, &fakeSearch{}, &fakeUsers{name: "admin", isAdmin: true})
	r := svc.Handle(context.Background(), Command{Platform: PlatformTelegram, ExternalID: "u1", Callback: "rej|r1"})
	if len(fr.rejected) != 1 || !strings.Contains(r.Text, "Rejected") {
		t.Fatalf("expected reject, got %q rejected=%v", r.Text, fr.rejected)
	}
}

func TestHandle_StatusListsRequests(t *testing.T) {
	st := newTestStore(t)
	link(t, st, PlatformTelegram, "u1", 7)
	fr := &fakeRequests{mine: []requests.Request{
		{Title: "Dune", Year: 2021, Status: requests.StatusPending},
		{Title: "Arrival", Year: 2016, Status: requests.StatusAvailable},
	}}
	svc := newTestService(t, st, fr, &fakeSearch{}, &fakeUsers{name: "alice"})
	r := svc.Handle(context.Background(), Command{Platform: PlatformTelegram, ExternalID: "u1", Text: "/status"})
	if !strings.Contains(r.Text, "Dune") || !strings.Contains(r.Text, "Arrival") {
		t.Fatalf("expected both titles, got %q", r.Text)
	}
}

func TestHandle_RateLimited(t *testing.T) {
	st := newTestStore(t)
	svc := newTestService(t, st, &fakeRequests{}, &fakeSearch{}, &fakeUsers{})
	var limited bool
	for i := 0; i < 40; i++ {
		r := svc.Handle(context.Background(), Command{Platform: PlatformTelegram, ExternalID: "spammer", Text: "/help"})
		if strings.Contains(r.Text, "going a bit fast") {
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("expected rate limiting to trigger")
	}
}

func TestStore_RedeemLinkCodeFlow(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	lc, err := st.CreateLinkCode(ctx, PlatformTelegram, "ext1", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(lc.Code) != codeLength {
		t.Fatalf("unexpected code length %d", len(lc.Code))
	}
	// Preview does not consume.
	if _, err := st.PreviewLinkCode(ctx, lc.Code); err != nil {
		t.Fatalf("preview: %v", err)
	}
	link, err := st.RedeemLinkCode(ctx, lc.Code, 42)
	if err != nil {
		t.Fatal(err)
	}
	if link.UserID != 42 || link.ExternalID != "ext1" {
		t.Fatalf("unexpected link %+v", link)
	}
	// Code is single-use.
	if _, err := st.RedeemLinkCode(ctx, lc.Code, 42); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("expected ErrInvalidCode on reuse, got %v", err)
	}
}

func TestStore_RedeemRejectsReassignToOtherUser(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	lc, _ := st.CreateLinkCode(ctx, PlatformTelegram, "ext1", "alice")
	if _, err := st.RedeemLinkCode(ctx, lc.Code, 1); err != nil {
		t.Fatal(err)
	}
	lc2, _ := st.CreateLinkCode(ctx, PlatformTelegram, "ext1", "alice")
	if _, err := st.RedeemLinkCode(ctx, lc2.Code, 2); !errors.Is(err, ErrLinkedToOther) {
		t.Fatalf("expected ErrLinkedToOther, got %v", err)
	}
	// Same user re-link is idempotent.
	lc3, _ := st.CreateLinkCode(ctx, PlatformTelegram, "ext1", "alice2")
	if _, err := st.RedeemLinkCode(ctx, lc3.Code, 1); err != nil {
		t.Fatalf("idempotent relink: %v", err)
	}
}

func TestStore_GenerateCodeIsCrockford(t *testing.T) {
	c, err := generateCode()
	if err != nil {
		t.Fatal(err)
	}
	for _, ch := range c {
		if !strings.ContainsRune(codeAlphabet, ch) {
			t.Fatalf("code char %q not in alphabet", string(ch))
		}
	}
}

// guard against accidental signature drift on the userID bridge.
var _ = strconv.Itoa
