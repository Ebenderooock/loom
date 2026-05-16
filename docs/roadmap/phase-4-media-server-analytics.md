# Phase 4: Media Server Analytics

### Goal
Integrate with Plex, Jellyfin, and Emby to provide watch history, playback statistics, active session monitoring, and usage analytics. Built-in Tautulli-like functionality. Also powers Phase 3's stale media detection and Phase 2's availability checking.

### Architecture Decisions
- **Data collection strategy:** WebSocket (primary real-time signal) + session polling (enrichment) + webhook receiver (optional). Follows Tautulli's proven approach. No PlexPass/Emby Premier required for core functionality (WebSocket works without it).
- **History storage:** Loom stores its own play history — doesn't rely on server-side history APIs (Jellyfin has no history endpoint, Emby is limited). Each play session is captured start-to-stop with full metadata.
- **Provider architecture:** `MediaServerProvider` interface with per-platform implementations (Plex, Jellyfin, Emby). Mirrors the download client `DownloadClient` interface + `RegisterKind` pattern.
- **Go libraries:** Use `LukeHagar/plexgo` for Plex (auto-generated from OpenAPI, broadest coverage). Write thin HTTP clients for Jellyfin/Emby (APIs are nearly identical, just prefix difference).
- **TMDB matching:** Plex uses `Guid` array (`tmdb://278`), Jellyfin/Emby use `ProviderIds` map (`{"Tmdb": "278"}`). Normalize to TMDB ID for cross-referencing with Loom's library.
- **Real-time updates:** WebSocket connections to each media server, maintained by a connection manager with auto-reconnect. Push active sessions to Loom's frontend via SSE/WebSocket.

### Patterns to follow
- `DownloadClient` interface + `RegisterKind` factory → `MediaServerProvider` + `RegisterProvider`
- `notifications.Dispatcher` subscription pattern → activity handler subscribes to media server events
- `scheduler.PeriodicScanner` → polling scheduler for session enrichment + history sync
- `eventbus.Bus` → publish `mediaserver.play.started`, `mediaserver.play.stopped`, `mediaserver.scrobble`

---

### Sub-phase 4A: Provider Interface + Core Infrastructure

#### Backend

1. **Migration: media server tables** — `internal/storage/migrations/sqlite/00XX_mediaservers.sql`
   ```sql
   CREATE TABLE media_server_connections (
     id TEXT PRIMARY KEY,
     name TEXT NOT NULL,
     type TEXT NOT NULL,                  -- 'plex' | 'jellyfin' | 'emby'
     url TEXT NOT NULL,                   -- base URL (e.g., http://192.168.1.10:32400)
     token TEXT NOT NULL,                 -- auth token or API key
     enabled BOOLEAN NOT NULL DEFAULT TRUE,
     settings JSONB,                      -- type-specific settings (plex client ID, jellyfin user ID, etc.)
     last_connected_at TIMESTAMP,
     last_error TEXT,
     created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
     updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
   );

   CREATE TABLE play_sessions (
     id TEXT PRIMARY KEY,
     server_id TEXT NOT NULL REFERENCES media_server_connections(id) ON DELETE CASCADE,
     session_key TEXT NOT NULL,           -- platform session ID
     user_id TEXT,                        -- platform user ID
     user_name TEXT,
     media_type TEXT NOT NULL,            -- 'movie' | 'episode' | 'track'
     tmdb_id INTEGER,
     loom_media_id TEXT,                  -- FK to movies/series if matched
     title TEXT NOT NULL,
     parent_title TEXT,                   -- show name for episodes
     grandparent_title TEXT,              -- series name
     year INTEGER,
     thumb_url TEXT,
     started_at TIMESTAMP NOT NULL,
     stopped_at TIMESTAMP,
     paused_duration INTEGER DEFAULT 0,   -- total seconds paused
     duration INTEGER NOT NULL,           -- total media duration (seconds)
     view_offset INTEGER DEFAULT 0,       -- how far they got (seconds)
     completed BOOLEAN DEFAULT FALSE,     -- watched > 85%
     play_method TEXT,                    -- 'direct_play' | 'direct_stream' | 'transcode'
     video_codec TEXT,
     audio_codec TEXT,
     video_decision TEXT,                 -- 'direct play' | 'copy' | 'transcode'
     audio_decision TEXT,
     stream_bitrate INTEGER,             -- kbps
     container TEXT,
     quality_profile TEXT,               -- e.g. "Original" or "4K 60Mbps"
     client_name TEXT,                   -- player app name
     device_name TEXT,
     platform TEXT,                      -- OS/device type
     ip_address TEXT,
     is_local BOOLEAN,
     created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
   );
   CREATE INDEX idx_play_sessions_server ON play_sessions(server_id);
   CREATE INDEX idx_play_sessions_user ON play_sessions(user_name);
   CREATE INDEX idx_play_sessions_tmdb ON play_sessions(tmdb_id);
   CREATE INDEX idx_play_sessions_started ON play_sessions(started_at);
   CREATE INDEX idx_play_sessions_media ON play_sessions(loom_media_id);

   CREATE TABLE media_server_users (
     id TEXT PRIMARY KEY,
     server_id TEXT NOT NULL REFERENCES media_server_connections(id) ON DELETE CASCADE,
     platform_user_id TEXT NOT NULL,
     username TEXT NOT NULL,
     thumb_url TEXT,
     is_admin BOOLEAN DEFAULT FALSE,
     last_seen_at TIMESTAMP,
     UNIQUE(server_id, platform_user_id)
   );

   CREATE TABLE media_server_libraries (
     id TEXT PRIMARY KEY,
     server_id TEXT NOT NULL REFERENCES media_server_connections(id) ON DELETE CASCADE,
     platform_library_id TEXT NOT NULL,
     name TEXT NOT NULL,
     type TEXT NOT NULL,                  -- 'movie' | 'show' | 'music' | 'photo'
     item_count INTEGER DEFAULT 0,
     last_synced_at TIMESTAMP,
     UNIQUE(server_id, platform_library_id)
   );
   ```

2. **Provider interface** — `internal/mediaservers/types.go`
   ```go
   type Provider interface {
       Type() ServerType                  // "plex", "jellyfin", "emby"
       Connect(ctx context.Context, cfg ConnectionConfig) error
       Test(ctx context.Context) error
       Libraries(ctx context.Context) ([]ServerLibrary, error)
       HasMedia(ctx context.Context, tmdbID int, mediaType string) (bool, error)
       Users(ctx context.Context) ([]ServerUser, error)
       ActiveSessions(ctx context.Context) ([]ActiveSession, error)
       PlayHistory(ctx context.Context, opts HistoryOpts) ([]HistoryEntry, error)
       SubscribeEvents(ctx context.Context, handler EventHandler) error  // WebSocket
       Close() error
   }
   type EventHandler func(event ServerEvent)
   ```
   - `ServerType` enum: `plex`, `jellyfin`, `emby`
   - `ActiveSession`: user, media, progress, transcode info, bandwidth, client
   - `HistoryEntry`: normalized play record (maps to `play_sessions` table)
   - `ServerEvent`: type (play/pause/stop/scrobble) + session data

3. **Provider registry** — `internal/mediaservers/registry.go`
   - `RegisterProvider(serverType, factory)` — factory pattern like download clients
   - `ProviderRegistry` — in-memory map of active provider instances with RWMutex
   - `Get(id)`, `List()`, `Register()`, `Remove()`

4. **sqlc queries** — `internal/storage/queries/sqlite/mediaservers.sql`
   - Connection CRUD: `CreateConnection`, `GetConnection`, `ListConnections`, `UpdateConnection`, `DeleteConnection`
   - Play sessions: `CreatePlaySession`, `UpdatePlaySession`, `ListPlaySessions` (with filters: user, media, date range, server)
   - Users: `UpsertUser`, `ListUsers`
   - Libraries: `UpsertLibrary`, `ListLibraries`
   - Analytics queries: `PlayCountByMedia`, `PlayCountByUser`, `PlaysByHour`, `TopMedia`, `RecentlyPlayed`, `WatchTimeByDay`

5. **Connection manager** — `internal/mediaservers/manager.go`
   - Manages lifecycle of all provider instances
   - On startup: load connections from DB → create providers → connect → subscribe events
   - Auto-reconnect on WebSocket disconnect (exponential backoff)
   - Health monitoring: periodic `Test()` calls, update `last_connected_at` / `last_error`
   - Hot-reload: add/remove/update connections without restart

---

### Sub-phase 4B: Platform Providers

6. **Plex provider** — `internal/mediaservers/plex/provider.go`
   - Uses `LukeHagar/plexgo` SDK
   - Auth: `X-Plex-Token` + identity headers
   - WebSocket: connect to `/:/websockets/notifications`, listen for `playing` events
   - On `playing` event → fetch `GET /status/sessions` for full session enrichment (Tautulli pattern)
   - Session state machine: track `playing` → `paused` → `stopped` transitions per sessionKey
   - `PlayHistory`: backfill from `GET /status/sessions/history/all` (Plex keeps indefinite history)
   - `HasMedia`: scan library sections, match `Guid` array for `tmdb://{id}`
   - `Users`: `GET /api/v2/users` via plex.tv

7. **Plex auth flow** — `internal/mediaservers/plex/auth.go`
   - PIN-based OAuth flow for setup UI:
     1. `POST plex.tv/api/v2/pins` → get code
     2. Frontend opens `app.plex.tv/auth#?clientID=X&code=Y`
     3. Backend polls `GET /api/v2/pins/{id}` until `authToken` is set
   - Token validation: `GET /api/v2/user` → confirms token + checks PlexPass status

8. **Jellyfin provider** — `internal/mediaservers/jellyfin/provider.go`
   - Thin HTTP client (no full SDK — `dweymouth/go-jellyfin` is music-focused)
   - Auth: `X-Emby-Token` or `Authorization: MediaBrowser Token="..."` header
   - WebSocket: connect to `/socket?api_key=TOKEN&deviceId=X`
   - Polling: `GET /Sessions?activeWithinSeconds=30` for session enrichment
   - No native history API → all history built from captured events
   - `HasMedia`: `GET /Items?AnyProviderIdEquals=Tmdb.{id}&Recursive=true`
   - `Users`: `GET /Users` (admin required)
   - Webhook receiver: accept `jellyfin-plugin-webhook` payloads (optional, enhances capture)

9. **Emby provider** — `internal/mediaservers/emby/provider.go`
   - Near-identical to Jellyfin with `/emby/` prefix on all paths
   - Auth: `X-Emby-Token` header
   - Same session/user/library endpoints
   - Native webhook support (Emby Premier) — same receiver endpoint, different payload format
   - Factor shared Jellyfin/Emby HTTP logic into `internal/mediaservers/mediabrowser/` base package

10. **Shared MediaBrowser base** — `internal/mediaservers/mediabrowser/client.go`
    - Shared HTTP client for Jellyfin + Emby (90% identical APIs)
    - Configurable path prefix (`""` for Jellyfin, `"/emby"` for Emby)
    - Auth header builder, session parser, user parser, library parser
    - WebSocket connection handler
    - Reduces code duplication significantly

---

### Sub-phase 4C: Analytics Engine

11. **Activity handler** — `internal/mediaservers/activity.go`
    - Receives `ServerEvent` from all providers
    - Session state machine: `started` → `playing` / `paused` → `stopped`
    - On `start`: create `play_sessions` row, publish `mediaserver.play.started` event
    - On `stop`: update row with final `view_offset`, `paused_duration`, `completed`, publish `mediaserver.play.stopped`
    - On scrobble (>85% watched): publish `mediaserver.scrobble` event
    - Match TMDB ID → Loom media ID for cross-referencing

12. **Watch state sync** — `internal/mediaservers/sync.go`
    - Subscribe to `mediaserver.scrobble` events
    - Update Loom movie/series "watched" state for auto-archive feature
    - Existing `auto_archive_watched` on libraries can now be powered by real data
    - Configurable: which servers to sync from, minimum completion %

13. **Analytics service** — `internal/mediaservers/analytics.go`
    - `Dashboard(ctx, timeRange)` → total plays, total watch time, unique users, concurrent peak
    - `TopMedia(ctx, timeRange, limit)` → most played movies/episodes
    - `LeastWatched(ctx, timeRange)` → downloaded but never/rarely played (feeds Phase 3 stale detection)
    - `UserStats(ctx, userID, timeRange)` → per-user watch time, play count, top genres
    - `PlaysByHour(ctx, timeRange)` → hourly distribution for peak time analysis
    - `WatchTimeTrend(ctx, days)` → daily watch time for trend charts
    - `TranscodeStats(ctx, timeRange)` → direct play vs transcode ratio, top transcoded codecs
    - `ClientBreakdown(ctx, timeRange)` → plays by client app/device/platform
    - `ConcurrentStreams(ctx, timeRange)` → peak concurrent streams over time

14. **Polling scheduler** — `internal/mediaservers/poller.go`
    - Periodic `ActiveSessions()` poll (every 5-10s) for servers where WebSocket isn't available or stable
    - Backfill: periodic `PlayHistory()` call for Plex to capture history from before Loom was connected
    - Library sync: periodic `Libraries()` + item count refresh
    - User sync: periodic `Users()` refresh

15. **Notification integration** — extend dispatcher
    - New events: `on_play_started`, `on_play_stopped`, `on_scrobble`
    - Optional: "Now Playing" feed to bots (Phase 2B `handleNowPlaying`)
    - `OnPlayback` boolean on notification `Connection`

16. **Availability checker integration** — connect to Phase 2
    - Implement `HasMedia(tmdbID, mediaType)` on the media server registry
    - Phase 2's availability checker calls this to check if media exists on connected servers
    - Returns: server name, library name, quality available

17. **Stale media integration** — connect to Phase 3
    - Expose `LeastWatched()` and per-media `LastWatchedDate()` APIs
    - Phase 3's stale media checker queries this to flag unwatched media
    - Threshold configurable in maintenance config

18. **API routes** — `internal/mediaservers/handlers.go`
    - **Connections:**
      - `POST   /api/v1/mediaservers` — add connection (with test)
      - `GET    /api/v1/mediaservers` — list connections with status
      - `GET    /api/v1/mediaservers/{id}` — connection detail
      - `PUT    /api/v1/mediaservers/{id}` — update
      - `DELETE /api/v1/mediaservers/{id}` — remove
      - `POST   /api/v1/mediaservers/{id}/test` — test connection
      - `GET    /api/v1/mediaservers/{id}/libraries` — list server libraries
      - `GET    /api/v1/mediaservers/{id}/users` — list server users
    - **Active sessions:**
      - `GET    /api/v1/mediaservers/sessions` — all active sessions across all servers
      - `GET    /api/v1/mediaservers/{id}/sessions` — sessions for one server
    - **History:**
      - `GET    /api/v1/mediaservers/history` — play history (filters: user, media, date range, server)
      - `GET    /api/v1/mediaservers/history/{id}` — single session detail
    - **Analytics:**
      - `GET    /api/v1/mediaservers/analytics/dashboard` — summary stats
      - `GET    /api/v1/mediaservers/analytics/top-media` — most watched
      - `GET    /api/v1/mediaservers/analytics/users` — per-user stats
      - `GET    /api/v1/mediaservers/analytics/activity` — plays by hour/day
      - `GET    /api/v1/mediaservers/analytics/transcode` — transcode breakdown
      - `GET    /api/v1/mediaservers/analytics/clients` — client/device breakdown
    - **Plex auth:**
      - `POST   /api/v1/mediaservers/plex/pin` — start PIN auth flow
      - `GET    /api/v1/mediaservers/plex/pin/{id}` — poll PIN status
    - **Webhooks:**
      - `POST   /api/v1/mediaservers/webhooks/plex` — Plex webhook receiver
      - `POST   /api/v1/mediaservers/webhooks/jellyfin` — Jellyfin webhook receiver
      - `POST   /api/v1/mediaservers/webhooks/emby` — Emby webhook receiver

#### Frontend

19. **Media server setup** — `web/src/pages/settings/mediaservers.tsx`
    - Add server: type selector (Plex/Jellyfin/Emby) → URL + token input → test → save
    - Plex: special PIN-based auth flow (opens plex.tv in popup, polls for token)
    - Connection cards with status indicator (connected/error/disconnected)
    - Library listing per server
    - User listing per server

20. **Active sessions** — `web/src/components/mediaservers/active-sessions.tsx`
    - Real-time grid of who's watching what (auto-refreshes)
    - Each card: user avatar, media poster, title, progress bar, play method badge (direct/transcode), client/device, bandwidth
    - Concurrent stream count header
    - Filterable by server

21. **Play history** — `web/src/pages/mediaservers/history.tsx`
    - Paginated table: date, user, media, duration watched, completion %, play method, client
    - Filters: date range, user, media type, server, completion status
    - Click to expand: full session detail (codecs, bitrate, IP, device)

22. **Analytics dashboard** — `web/src/pages/mediaservers/analytics.tsx`
    - **Summary cards:** total plays (period), total watch time, unique users, peak concurrent
    - **Watch time trend:** line/area chart showing daily watch hours
    - **Most watched:** ranked list with poster, play count, total watch time
    - **Activity heatmap:** plays by day-of-week × hour-of-day
    - **User leaderboard:** top users by watch time
    - **Transcode pie chart:** direct play vs direct stream vs transcode
    - **Client breakdown:** bar chart by app/platform
    - Time range selector: 7d / 30d / 90d / 1y / all

23. **Media detail integration** — extend movie/series detail views
    - Play count badge on movie/series cards
    - "Last watched" and "Watch count" on detail pages
    - Per-user watch history for the specific media item

24. **Now Playing widget** — `web/src/components/mediaservers/now-playing-widget.tsx`
    - Compact widget for dashboard sidebar
    - Shows active stream count + mini cards
    - Real-time updates via SSE or WebSocket

---

### Implementation Order

```
4A.1-4   Schema + interface + registry + queries    ─┐ Core foundation
4A.5     Connection manager                          ─┘
4B.10    Shared MediaBrowser base                    ── before Jellyfin/Emby
4B.6-7   Plex provider + auth                       ── can parallelize with ↓
4B.8     Jellyfin provider                           ── requires MediaBrowser base
4B.9     Emby provider                               ── requires MediaBrowser base
4C.11    Activity handler                            ── requires providers
4C.12    Watch state sync                            ── requires activity handler
4C.13    Analytics service                           ── requires play_sessions data
4C.14    Polling scheduler                           ── requires providers
4C.15    Notification integration                    ── requires activity handler
4C.16-17 Phase 2/3 integrations                      ── requires providers + analytics
4C.18    API routes                                  ── requires all services
4C.19-24 Frontend                                    ── requires API
```

### Dependencies
- Existing `eventbus.Bus` for play events
- Existing `movies.Service` / `series.Service` for TMDB ID cross-referencing
- Phase 2 (availability checker stub) → now fully implemented
- Phase 3 (stale media checker stub) → now fully implemented
- `LukeHagar/plexgo` Go dependency for Plex
- No external dependencies for Jellyfin/Emby (thin HTTP clients)

### Risks
- **Plex WebSocket stability:** Long-lived WebSocket connections can drop. Need robust reconnection with exponential backoff.
- **Jellyfin no history API:** All history must be captured in real-time. If Loom is down, play sessions are missed. Mitigate with polling fallback + activity log parsing.
- **Plex auth complexity:** PIN flow requires user interaction (browser redirect). UX must be smooth.
- **Data volume:** Active Plex servers can generate thousands of play sessions/month. Need efficient pagination and optional data retention policies.
- **TMDB matching performance:** Scanning entire Plex libraries to match by GUID is slow. Cache the mapping and refresh periodically.
- **Emby Premier requirement:** Webhook support needs paid tier. Polling fallback works but is less real-time.
