# Phase 2: Media Requests

### Goal
Built-in media request system (replaces Overseerr/Jellyseerr) with multi-user support, guest access, interactive bots (Discord/Telegram/Slack/WhatsApp), auto-approval engine, media server availability checking, and Overseerr API compatibility.

### Architecture Decisions
- **User model:** Multi-user + guest + bot. Authenticated users and unauthenticated guests can request. Bots submit requests on behalf of chat users.
- **Auto-approval:** Fully configurable — per-user quotas, role-based rules, content rating filters, media type rules.
- **Availability checking:** Check both Loom library AND connected media servers (Plex/Jellyfin/Emby) before accepting requests.
- **Bots:** Built-in modular handlers inside binary (mirrors notification `Sender` pattern). Future migration path to Script Engine (Phase 5).
- **Overseerr compat:** Expose `/api/v1/request` endpoints matching Overseerr's API schema so existing tools work with Loom.
- **Patterns to follow:**
  - `Sender` interface pattern → `BotAdapter` interface for chat platforms
  - `RegisterKind` factory → `RegisterBotPlatform` for bot registration
  - `eventbus.Bus` → new request event topics
  - `goose` migrations + `sqlc` queries for persistence
  - `chi` router for API endpoints
  - `IdentityFrom(ctx)` for user context

---

### Sub-phase 2A: Core Request Engine

#### Backend

1. **Migration: `requests` table** — `internal/storage/migrations/sqlite/00XX_requests.sql`
   ```sql
   CREATE TABLE requests (
     id TEXT PRIMARY KEY,
     media_type TEXT NOT NULL,        -- 'movie' | 'series'
     tmdb_id INTEGER NOT NULL,
     tvdb_id INTEGER,
     title TEXT NOT NULL,
     year INTEGER,
     poster_url TEXT,
     overview TEXT,
     requester_id TEXT,               -- FK to users.id (NULL for guests)
     requester_name TEXT NOT NULL,     -- display name (guest or user)
     requester_source TEXT NOT NULL,   -- 'web' | 'api' | 'discord' | 'telegram' | 'slack' | 'whatsapp' | 'overseerr'
     status TEXT NOT NULL DEFAULT 'pending',  -- pending | approved | declined | available | unavailable
     status_reason TEXT,              -- admin note on decline, auto-approve rule name, etc.
     media_id TEXT,                   -- FK to movies.id or series.id once fulfilled
     priority INTEGER DEFAULT 0,
     requested_seasons TEXT,          -- JSON array for series (NULL = all)
     created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
     updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
     fulfilled_at TIMESTAMP,
     UNIQUE(media_type, tmdb_id, requester_id)
   );
   CREATE TABLE request_comments (
     id TEXT PRIMARY KEY,
     request_id TEXT NOT NULL REFERENCES requests(id) ON DELETE CASCADE,
     author_id TEXT,
     author_name TEXT NOT NULL,
     body TEXT NOT NULL,
     created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
   );
   CREATE TABLE request_history (
     id TEXT PRIMARY KEY,
     request_id TEXT NOT NULL REFERENCES requests(id) ON DELETE CASCADE,
     field TEXT NOT NULL,             -- 'status', 'priority', etc.
     old_value TEXT,
     new_value TEXT,
     changed_by TEXT,
     changed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
   );
   ```

2. **Request types** — `internal/requests/types.go`
   - `Request` struct matching the table
   - `RequestStatus` enum: `pending`, `approved`, `declined`, `available`, `unavailable`
   - `RequesterSource` enum: `web`, `api`, `discord`, `telegram`, `slack`, `whatsapp`, `overseerr`
   - `MediaType` enum: `movie`, `series`
   - `CreateRequest`, `UpdateRequest`, `RequestFilter` DTOs

3. **sqlc queries** — `internal/storage/queries/sqlite/requests.sql`
   - `CreateRequest`, `GetRequest`, `ListRequests` (with filters/pagination), `UpdateRequestStatus`, `DeleteRequest`
   - `CountRequestsByUser` (for quota), `GetRequestByTMDB` (duplicate check)
   - `CreateComment`, `ListComments`, `CreateHistory`, `ListHistory`

4. **Request service** — `internal/requests/service.go`
   - `Create(ctx, req)` → duplicate check → availability check → auto-approve evaluation → persist → publish event → return
   - `Approve(ctx, id, approverID)` → update status → trigger fulfillment → publish event
   - `Decline(ctx, id, approverID, reason)` → update status → publish event
   - `List(ctx, filter)` → filtered/paginated list
   - `Get(ctx, id)` → single request with comments/history
   - `Delete(ctx, id)` → soft delete
   - `BulkApprove(ctx, ids)` / `BulkDecline(ctx, ids, reason)`

5. **Auto-approval engine** — `internal/requests/approval.go`
   - `ApprovalRule` type: condition + action
   - Conditions: user role, user ID, media type, content rating (e.g., PG-13 and below), genre, request count in period
   - `Evaluate(ctx, req, user) → (approved bool, ruleName string)`
   - Rules stored in appconfig (hot-reloadable)
   - Default: all requests require manual approval

6. **Quota system** — `internal/requests/quota.go`
   - Per-user configurable limits: X requests per day/week/month
   - Guest quota separate from authenticated user quota
   - Admin bypass
   - `CheckQuota(ctx, userID) → (remaining int, err error)`

7. **Availability checker** — `internal/requests/availability.go`
   - Check Loom library: query `movies` / `series` tables by TMDB ID
   - Check media servers: use `internal/mediaservers/` interfaces (Phase 4, stub initially)
   - Returns: `not_found`, `in_library`, `available_on_server`, `already_requested`
   - If already available → auto-mark request as `available` with a message

8. **Fulfillment bridge** — `internal/requests/fulfillment.go`
   - On approval: call `movies.Service.AddMovie()` or `series.Service.AddSeries()` with TMDB ID
   - Subscribe to movie/series status change events on the event bus
   - When media reaches `available_right_quality` or `available_higher_quality` → update request to `available`
   - Wire: `SubscribeToFulfillment(bus, requestSvc, movieSvc, seriesSvc)`

9. **Notification integration** — `internal/requests/events.go`
   - New event types: `on_request`, `on_request_approved`, `on_request_declined`, `on_request_available`
   - Add `OnRequest`, `OnRequestApproved`, `OnRequestDeclined`, `OnRequestAvailable` booleans to `Connection` / `ConnectionSettings`
   - New migration to add columns to `notification_connections`
   - Extend `dispatcher.go` subscriptions to handle request events

10. **API routes** — `internal/requests/handlers.go`
    - `POST   /api/v1/requests` — create (auth: any user or guest)
    - `GET    /api/v1/requests` — list (auth: own requests for users, all for admin)
    - `GET    /api/v1/requests/{id}` — detail with comments/history
    - `PUT    /api/v1/requests/{id}/approve` — approve (auth: admin)
    - `PUT    /api/v1/requests/{id}/decline` — decline with reason (auth: admin)
    - `DELETE /api/v1/requests/{id}` — delete (auth: owner or admin)
    - `POST   /api/v1/requests/{id}/comments` — add comment (auth: any)
    - `POST   /api/v1/requests/bulk/approve` — bulk approve (auth: admin)
    - `POST   /api/v1/requests/bulk/decline` — bulk decline (auth: admin)
    - `GET    /api/v1/requests/count` — counts by status (for dashboard)
    - `GET    /api/v1/requests/quota` — current user's remaining quota

11. **Guest identity** — `internal/auth/guest.go`
    - Extend `resolveIdentity` to support guest requests: no auth required on `POST /api/v1/requests` but rate-limited
    - Guest identity has `role: "guest"`, no user ID, identified by IP + optional name/email field
    - Rate limiting: configurable per-IP limit (default: 5 requests/day)

#### Frontend

12. **Requests page** — `web/src/pages/requests.tsx`
    - Tab bar: All / Pending / Approved / Available / Declined
    - Request cards with poster, title, year, requester, status badge, approve/decline buttons (admin)
    - Search/filter bar
    - Bulk selection + actions

13. **Request dialog** — `web/src/components/requests/request-dialog.tsx`
    - TMDB search (reuse existing lookup endpoints)
    - Show availability status before submitting
    - Season selection for series
    - Optional note/comment field
    - Accessible from: movies page, series page, dedicated request page, navbar quick-add

14. **Request status on media cards** — `web/src/components/movies/movie-card.tsx` + `series/`
    - Show "Requested" badge on movie/series cards if a pending request exists
    - Show requester info on detail views

15. **Request notifications** — toast on approval/decline (via existing notification websocket)

16. **Request settings** — `web/src/pages/settings/requests.tsx`
    - Auto-approval rules builder (conditions + actions)
    - Quota configuration per role
    - Guest request toggle + rate limit
    - Default quality profile for fulfilled requests

---

### Sub-phase 2B: Interactive Bots

#### Architecture

Bot system mirrors the notification `Sender` pattern:

```
internal/bots/
├── types.go          # BotAdapter interface, Command, Response types
├── registry.go       # RegisterPlatform factory, BotRegistry (like download Registry)
├── router.go         # Command router: parse input → find handler → execute → format response
├── handlers.go       # Shared command handlers (search, request, approve, status, etc.)
├── service.go        # Bot service: lifecycle, config, webhook receiver
├── discord/
│   ├── adapter.go    # Discord slash command registration, interaction handler
│   └── formatter.go  # Discord embeds, buttons, select menus
├── telegram/
│   ├── adapter.go    # Telegram bot API, command handler, callback queries
│   └── formatter.go  # Telegram inline keyboards, formatted messages
├── slack/
│   ├── adapter.go    # Slack app, slash commands, interactive messages
│   └── formatter.go  # Slack blocks, actions
└── whatsapp/
    ├── adapter.go    # WhatsApp Business API, message handler
    └── formatter.go  # WhatsApp template messages, list messages
```

#### Backend

17. **BotAdapter interface** — `internal/bots/types.go`
    ```go
    type BotAdapter interface {
        Platform() string
        Start(ctx context.Context, cfg BotConfig) error
        Stop(ctx context.Context) error
        HandleWebhook(w http.ResponseWriter, r *http.Request)  // incoming events
        SendMessage(ctx context.Context, channelID string, msg Response) error
    }
    ```

18. **Command model** — `internal/bots/types.go`
    - `Command` struct: `Name`, `Args`, `UserID`, `ChannelID`, `Platform`, `RawMessage`
    - `Response` struct: `Text`, `Embeds`, `Actions` (buttons/menus), `Ephemeral`
    - Commands: `/request <query>`, `/search <query>`, `/status [id]`, `/myrequests`, `/approve <id>`, `/deny <id> [reason]`, `/nowplaying`, `/recent`

19. **Command router** — `internal/bots/router.go`
    - Parse platform-agnostic command from adapter input
    - Route to shared handler
    - Handler returns platform-agnostic `Response`
    - Adapter formats response for its platform (embeds, keyboards, etc.)

20. **Shared handlers** — `internal/bots/handlers.go`
    - `handleSearch(ctx, query)` → call movies/series lookup → return results with "Request" action buttons
    - `handleRequest(ctx, tmdbID, mediaType, user)` → call `requests.Service.Create()` → return confirmation
    - `handleStatus(ctx, requestID)` → call `requests.Service.Get()` → return status
    - `handleMyRequests(ctx, userID)` → call `requests.Service.List(filter: userID)` → return list
    - `handleApprove(ctx, requestID, adminID)` → call `requests.Service.Approve()` → return confirmation
    - `handleDeny(ctx, requestID, adminID, reason)` → call `requests.Service.Decline()` → return confirmation
    - `handleNowPlaying(ctx)` → call media server active sessions (Phase 4 stub)
    - `handleRecent(ctx)` → query recently added movies/series

21. **Bot user mapping** — `internal/bots/identity.go`
    - Map chat platform user IDs to Loom users (optional linking)
    - Unlinked users treated as guests with platform-specific identity (`discord:12345`, `telegram:67890`)
    - `/link` command to associate chat account with Loom account

22. **Discord adapter** — `internal/bots/discord/adapter.go`
    - Uses Discord Interactions API (webhook-based, no gateway connection needed)
    - Registers slash commands on startup via REST API
    - Handles interaction webhooks at `/api/v1/bots/discord/webhook`
    - Rich embeds with poster images, buttons for approve/deny, select menus for search results

23. **Telegram adapter** — `internal/bots/telegram/adapter.go`
    - Uses Telegram Bot API with webhook mode
    - Registers commands on startup
    - Handles updates at `/api/v1/bots/telegram/webhook`
    - Inline keyboards for actions, callback queries for button presses

24. **Slack adapter** — `internal/bots/slack/adapter.go`
    - Uses Slack Bolt/Events API
    - Slash commands + interactive messages
    - Handles events at `/api/v1/bots/slack/webhook`
    - Block Kit for rich message formatting

25. **WhatsApp adapter** — `internal/bots/whatsapp/adapter.go`
    - Uses WhatsApp Business Cloud API (Meta)
    - Message-based commands (no slash commands)
    - Handles webhooks at `/api/v1/bots/whatsapp/webhook`
    - List messages for search results, template messages for notifications

26. **Bot config & persistence** — migration + appconfig
    - Per-platform: enabled, token/credentials, webhook URL, allowed channels, admin channel
    - Stored in appconfig (hot-reloadable)

27. **Bot API routes** — `internal/bots/handlers_api.go`
    - `GET    /api/v1/bots` — list configured bots with status
    - `PUT    /api/v1/bots/{platform}` — update bot config
    - `POST   /api/v1/bots/{platform}/test` — send test message
    - `DELETE /api/v1/bots/{platform}` — disable bot
    - Webhook endpoints per platform (see above)

28. **Bot → notification bridge** — bots can also send proactive messages
    - When a request is approved/declined/available, notify the requester via their originating platform
    - Subscribe to request events on the event bus
    - Look up requester's platform + channel → send DM or channel message

#### Frontend

29. **Bot settings page** — `web/src/pages/settings/bots.tsx`
    - Per-platform card: enable/disable, token input, webhook URL display, test button
    - Linked users management
    - Allowed channels configuration

30. **Bot activity log** — `web/src/components/bots/activity-log.tsx`
    - Recent commands received, responses sent, errors
    - Filterable by platform

---

### Sub-phase 2C: Overseerr API Compatibility

31. **Overseerr compat routes** — `internal/requests/overseerr.go`
    - Mount at `/api/v1/request` (Overseerr's endpoint)
    - Map Overseerr request schema ↔ Loom request schema
    - Support: create, approve, decline, list, status
    - Overseerr media types: `movie` (1), `tv` (2)
    - Auth: Overseerr API key header (`X-Api-Key`)

32. **Overseerr user mapping** — map Overseerr user IDs to Loom users or treat as external requesters

33. **Overseerr search compat** — `/api/v1/search` endpoint matching Overseerr's search API (proxies to TMDB)

34. **Overseerr status endpoint** — `/api/v1/status` returning Loom version in Overseerr-compatible format

---

### Sub-phase 2D: Media Server Availability (Plex/Jellyfin/Emby stubs)

35. **Media server interface** — `internal/mediaservers/types.go`
    - `MediaServer` interface with `HasMedia(tmdbID, mediaType) (bool, error)` method
    - This is a **stub** for Phase 2 — full implementation comes in Phase 4
    - Availability checker calls registered media servers

36. **Media server config** — migration + appconfig for server connection details (URL, token)

---

### Implementation Order

```
2A.1-3   Schema + types + queries          ─┐
2A.4     Request service                    ─┤ Core foundation
2A.5-6   Approval engine + quotas           ─┤
2A.7     Availability checker (Loom-only)   ─┘
2A.8     Fulfillment bridge                 ── requires movie/series services
2A.9     Notification integration           ── requires event bus
2A.10-11 API routes + guest identity        ── requires service + auth
2A.12-16 Frontend                           ── requires API
2B.17-21 Bot framework + shared handlers    ── requires request service
2B.22-25 Platform adapters                  ── requires bot framework (can parallelize)
2B.26-30 Bot config + frontend              ── requires adapters
2C.31-34 Overseerr compat                   ── requires request service
2D.35-36 Media server stubs                 ── independent
```

### Dependencies
- Existing `movies.Service` and `series.Service` for fulfillment
- Existing `eventbus.Bus` for events
- Existing `auth` middleware for user context
- Existing TMDB integration for search
- Phase 4 (Media Server Analytics) for full availability checking — stubbed initially

### Risks
- **Bot token management:** Each platform has different auth flows (Discord OAuth, Telegram BotFather, Slack App, WhatsApp Business). Need clear setup docs.
- **WhatsApp complexity:** Business API requires Meta business verification — may be low-priority.
- **Overseerr compat scope:** Overseerr's API is large — only implement the request-critical subset.
- **Guest abuse:** Rate limiting and CAPTCHA may be needed for public-facing request endpoints.
- **Bot user linking:** Mapping chat users to Loom users adds UX complexity — start with unlinked (guest) mode.
