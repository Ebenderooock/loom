// Typed fetch wrappers + react-query hooks for the Loom music REST endpoints.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/lib/fetch";

// ---------- Types ----------

export type MonitoringStatus = "monitored" | "unmonitored";

export interface ArtistStats {
  albumCount: number;
  monitoredAlbumCount: number;
  trackCount: number;
  trackFileCount: number;
  missingTrackCount: number;
}

export interface Track {
  id: string;
  album_id: string;
  release_id?: string;
  title?: string;
  track_number: number;
  disc_number: number;
  duration_ms?: number;
  artist_name?: string;
  monitored: boolean;
  has_file: boolean;
}

export interface AlbumRelease {
  id: string;
  mbid: string;
  album_id: string;
  title?: string;
  disambiguation?: string;
  status?: string;
  release_date?: string;
  country?: string;
  label?: string;
  format?: string;
  media_count: number;
  track_count: number;
}

export type ExternalLinkType =
  | "website"
  | "spotify"
  | "apple_music"
  | "youtube_music"
  | "tidal"
  | "deezer"
  | "amazon_music"
  | "bandcamp"
  | "soundcloud"
  | "youtube"
  | "wikipedia"
  | "wikidata"
  | "discogs"
  | "allmusic"
  | "lastfm"
  | "instagram"
  | "facebook"
  | "twitter"
  | "tiktok";

export interface ExternalLink {
  type: ExternalLinkType;
  url: string;
}

export interface Album {
  id: string;
  mbid: string;
  artist_id?: string;
  title: string;
  album_type?: string;
  secondary_types?: string[];
  release_date?: string;
  genres?: string[];
  cover_art_url?: string;
  overview?: string;
  rating?: number;
  rating_votes?: number;
  links?: ExternalLink[];
  monitored: boolean;
  selected_release_id?: string;
  library_id?: string;
  quality_profile_id?: string;
  metadata_profile_id?: string;
  last_search_at?: string;
  releases_fetched_at?: string;
  tracks_fetched_at?: string;
  details_fetched_at?: string;
  created_at?: string;
  updated_at?: string;
  artists?: AlbumArtist[];
  releases?: AlbumRelease[];
  tracks?: Track[];
}

export interface AlbumArtist {
  artist_id?: string;
  mbid?: string;
  name: string;
  join_phrase?: string;
}

export function formatAlbumArtistCredits(
  artists: AlbumArtist[] | undefined,
): string {
  if (!artists?.length) return "Various Artists";
  return artists
    .map((artist, index) => {
      const phrase =
        artist.join_phrase ?? (index < artists.length - 1 ? ", " : "");
      return `${artist.name}${phrase}`;
    })
    .join("")
    .trimEnd();
}

export interface AlbumLookupResult {
  mbid: string;
  title: string;
  artists?: AlbumArtist[];
  album_type?: string;
  release_date?: string;
  cover_art_url?: string;
  already_added: boolean;
}

export interface AddAlbumRequest {
  mbid: string;
  qualityProfileId: string;
  libraryId: string;
  metadataProfileId?: string;
  monitored?: boolean;
}

export interface Artist {
  id: string;
  mbid: string;
  name: string;
  sort_name?: string;
  disambiguation?: string;
  artist_type?: string;
  country?: string;
  overview?: string;
  genres?: string[];
  image_url?: string;
  background_url?: string;
  logo_url?: string;
  begin_area?: string;
  begin_date?: string;
  end_date?: string;
  ended?: boolean;
  rating?: number;
  rating_votes?: number;
  links?: ExternalLink[];
  path?: string;
  library_id?: string;
  quality_profile_id?: string;
  metadata_profile_id?: string;
  monitoring_status: MonitoringStatus;
  metadata_provider?: string;
  last_search_at?: string;
  created_at: string;
  updated_at: string;
  albums?: Album[];
  stats?: ArtistStats;
}

export interface UpdateArtistRequest {
  monitoringStatus?: MonitoringStatus;
  qualityProfileId?: string;
  metadataProfileId?: string;
  libraryId?: string;
  path?: string;
}

export interface ArtistFile {
  id: string;
  track_id?: string;
  album_id?: string;
  artist_id?: string;
  file_path: string;
  size: number;
  quality?: string;
  format?: string;
  bitrate?: number;
  date_added: string;
  album_title?: string;
  track_title?: string;
  track_number?: number;
  disc_number?: number;
}

export interface ArtistHistoryEntry {
  id: string;
  type: "grab" | "import";
  status: string;
  album_id?: string;
  album_title?: string;
  title?: string;
  reason?: string;
  source_path?: string;
  dest_path?: string;
  date: string;
}

export interface ArtistLookupResult {
  mbid: string;
  name: string;
  disambiguation?: string;
  type?: string;
  country?: string;
  genres?: string[];
  image_url?: string;
  already_added: boolean;
}

export interface AddArtistRequest {
  mbid: string;
  qualityProfileId: string;
  libraryId: string;
  metadataProfileId?: string;
  monitoringStatus?: MonitoringStatus;
  search?: boolean;
}

export interface AudioQualityDefinition {
  id: string;
  name: string;
  format?: string;
  bitrate?: number;
  vbr: boolean;
  lossless: boolean;
  tier_order: number;
}

export interface AudioFormatItem {
  format_id: string;
  score: number;
}

export interface AudioQualityProfile {
  id: string;
  name: string;
  items: unknown;
  cutoff?: string;
  upgrade_allowed: boolean;
  format_items?: AudioFormatItem[];
  min_format_score?: number;
}

export interface CreateAudioQualityProfileRequest {
  name: string;
  items: { definition_id: string; allowed: boolean }[];
  cutoff?: string;
  upgrade_allowed?: boolean;
}

export interface MetadataProfile {
  id: string;
  name: string;
  primary_types: string[];
  secondary_types: string[];
  release_statuses: string[];
}

export interface CreateMetadataProfileRequest {
  name: string;
  primary_types: string[];
  secondary_types: string[];
  release_statuses: string[];
}

export interface UpdateMetadataProfileRequest {
  name?: string;
  primary_types?: string[];
  secondary_types?: string[];
  release_statuses?: string[];
}

export interface AlbumGrabResult {
  album_id: string;
  title: string;
  indexer_id: string;
  size: number;
  quality_name: string;
  tier: number;
  composite_score: number;
  client_id: string;
  download_id: string;
}

export interface ReleaseCandidate {
  guid: string;
  title: string;
  indexer_id: string;
  size: number;
  seeders?: number;
  protocol: string;
  quality_name: string;
  tier: number;
  score: number;
  format_score?: number;
  allowed: boolean;
  meets_cutoff: boolean;
  link?: string;
  magnet_uri?: string;
  infohash?: string;
}

// ---------- Query keys ----------

export const musicKeys = {
  artists: ["music", "artists"] as const,
  albums: ["music", "albums"] as const,
  artist: (id: string) => ["music", "artist", id] as const,
  artistFiles: (id: string) => ["music", "artist", id, "files"] as const,
  artistHistory: (id: string) => ["music", "artist", id, "history"] as const,
  album: (id: string) => ["music", "album", id] as const,
  lookup: (q: string) => ["music", "lookup", q] as const,
  audioQualityDefinitions: ["music", "audio-quality-definitions"] as const,
  audioQualityProfiles: ["music", "audio-quality-profiles"] as const,
  metadataProfiles: ["music", "metadata-profiles"] as const,
  metadataSource: ["music", "metadata-source"] as const,
  artworkSettings: ["music", "artwork-settings"] as const,
};

export interface ArtworkSettings {
  fanart_api_key_set: boolean;
  audiodb_api_key_set: boolean;
  fanart_api_key_from_env: boolean;
  audiodb_api_key_from_env: boolean;
}

export interface ArtworkSettingsUpdate {
  // Empty keeps the stored value unless the matching clear flag is true.
  fanart_api_key?: string;
  audiodb_api_key?: string;
  clear_fanart_api_key?: boolean;
  clear_audiodb_api_key?: boolean;
}

export interface ArtworkTestResult {
  ok: boolean;
  fanart_tested: boolean;
  fanart_ok: boolean;
  fanart_error?: string;
  audiodb_ok: boolean;
  audiodb_error?: string;
  audiodb_shared_key: boolean;
}

export type MetadataSourceMode = "musicbrainz" | "custom";

export interface MetadataSource {
  mode: MetadataSourceMode;
  base_url: string;
  rate_limit_ms: number;
  auth_header_name: string;
  auth_header_set: boolean;
  search_on_public: boolean;
}

export interface MetadataSourceUpdate {
  mode: MetadataSourceMode;
  base_url: string;
  rate_limit_ms: number;
  auth_header_name: string;
  // Empty keeps the stored value unless clear_auth_header is true.
  auth_header_value?: string;
  clear_auth_header?: boolean;
  search_on_public: boolean;
}

export interface MetadataSourceTestResult {
  ok: boolean;
  lookup_ok: boolean;
  lookup_error?: string;
  search_tested: boolean;
  search_ok: boolean;
  search_error?: string;
}

// ---------- Fetchers ----------

async function getJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  const res = await apiFetch(path, { signal });
  if (!res.ok) {
    throw new Error(`${path}: ${res.status} ${res.statusText}`);
  }
  return (await res.json()) as T;
}

export async function fetchArtists(signal?: AbortSignal): Promise<Artist[]> {
  return getJSON<Artist[]>("/api/v1/artists", signal);
}

export async function fetchAlbums(signal?: AbortSignal): Promise<Album[]> {
  return getJSON<Album[]>("/api/v1/albums", signal);
}

export async function fetchArtist(
  id: string,
  signal?: AbortSignal,
): Promise<Artist> {
  return getJSON<Artist>(`/api/v1/artists/${id}`, signal);
}

export async function fetchAlbum(
  id: string,
  signal?: AbortSignal,
): Promise<Album> {
  return getJSON<Album>(`/api/v1/albums/${id}`, signal);
}

export async function lookupAlbums(
  q: string,
  signal?: AbortSignal,
): Promise<AlbumLookupResult[]> {
  if (!q.trim()) return [];
  return getJSON<AlbumLookupResult[]>(
    `/api/v1/albums/lookup?q=${encodeURIComponent(q)}`,
    signal,
  );
}

export async function addAlbum(req: AddAlbumRequest): Promise<Album> {
  const res = await apiFetch("/api/v1/albums", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
  if (!res.ok) {
    throw new Error(`add album failed: ${res.status} ${await res.text()}`);
  }
  return (await res.json()) as Album;
}

export async function lookupArtists(
  q: string,
  signal?: AbortSignal,
): Promise<ArtistLookupResult[]> {
  if (!q.trim()) return [];
  return getJSON<ArtistLookupResult[]>(
    `/api/v1/artists/lookup?q=${encodeURIComponent(q)}`,
    signal,
  );
}

export async function addArtist(req: AddArtistRequest): Promise<Artist> {
  const res = await apiFetch("/api/v1/artists", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
  if (!res.ok) {
    throw new Error(`add artist failed: ${res.status} ${await res.text()}`);
  }
  return (await res.json()) as Artist;
}

export async function deleteArtist(id: string): Promise<void> {
  const res = await apiFetch(`/api/v1/artists/${id}`, { method: "DELETE" });
  if (!res.ok && res.status !== 204) {
    throw new Error(`delete artist failed: ${res.status}`);
  }
}

export async function setArtistMonitoring(
  id: string,
  status: MonitoringStatus,
): Promise<Artist> {
  const res = await apiFetch(`/api/v1/artists/${id}/monitoring`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ status }),
  });
  if (!res.ok) throw new Error(`set monitoring failed: ${res.status}`);
  return (await res.json()) as Artist;
}

export async function refreshArtist(id: string): Promise<Artist> {
  const res = await apiFetch(`/api/v1/artists/${id}/refresh`, {
    method: "POST",
  });
  if (!res.ok) {
    const detail = (await res.text()).trim();
    throw new Error(detail || `refresh failed: ${res.status}`);
  }
  return (await res.json()) as Artist;
}

export async function rescanArtist(
  id: string,
  libraryId: string,
): Promise<void> {
  const res = await apiFetch(`/api/v1/artists/${id}/rescan`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ libraryId }),
  });
  if (!res.ok) throw new Error(`rescan artist failed: ${res.status}`);
}

export async function setAlbumMonitored(
  id: string,
  monitored: boolean,
): Promise<Album> {
  const res = await apiFetch(`/api/v1/albums/${id}/monitoring`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ monitored }),
  });
  if (!res.ok) throw new Error(`set album monitored failed: ${res.status}`);
  return (await res.json()) as Album;
}

export async function setTrackMonitored(
  albumId: string,
  trackId: string,
  monitored: boolean,
): Promise<Track> {
  const res = await apiFetch(
    `/api/v1/albums/${albumId}/tracks/${trackId}/monitoring`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ monitored }),
    },
  );
  if (!res.ok) throw new Error(`set track monitored failed: ${res.status}`);
  return (await res.json()) as Track;
}

export async function updateArtist(
  id: string,
  req: UpdateArtistRequest,
): Promise<Artist> {
  const res = await apiFetch(`/api/v1/artists/${id}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
  if (!res.ok) {
    const detail = (await res.text()).trim();
    throw new Error(detail || `update artist failed: ${res.status}`);
  }
  return (await res.json()) as Artist;
}

export async function fetchArtistFiles(
  id: string,
  signal?: AbortSignal,
): Promise<ArtistFile[]> {
  return getJSON<ArtistFile[]>(`/api/v1/artists/${id}/files`, signal);
}

export async function fetchArtistHistory(
  id: string,
  signal?: AbortSignal,
): Promise<ArtistHistoryEntry[]> {
  return getJSON<ArtistHistoryEntry[]>(`/api/v1/artists/${id}/history`, signal);
}

export async function selectAlbumRelease(
  albumId: string,
  releaseId: string,
): Promise<Album> {
  const res = await apiFetch(`/api/v1/albums/${albumId}/release`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ release_id: releaseId }),
  });
  if (!res.ok) {
    throw new Error(`select album release failed: ${res.status}`);
  }
  return (await res.json()) as Album;
}

export interface UpdateAudioQualityProfileRequest {
  cutoff?: string;
  upgrade_allowed?: boolean;
  items?: { definition_id: string; allowed: boolean }[];
  format_items?: AudioFormatItem[];
  min_format_score?: number;
}

export async function updateAudioQualityProfile(
  id: string,
  req: UpdateAudioQualityProfileRequest,
): Promise<AudioQualityProfile> {
  const res = await apiFetch(`/api/v1/music/audio-quality-profiles/${id}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
  if (!res.ok)
    throw new Error(`update audio quality profile failed: ${res.status}`);
  return (await res.json()) as AudioQualityProfile;
}

export async function createAudioQualityProfile(
  req: CreateAudioQualityProfileRequest,
): Promise<AudioQualityProfile> {
  const res = await apiFetch("/api/v1/music/audio-quality-profiles", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
  if (!res.ok) {
    throw new Error(`create audio quality profile failed: ${res.status}`);
  }
  return (await res.json()) as AudioQualityProfile;
}

export async function deleteAudioQualityProfile(id: string): Promise<void> {
  const res = await apiFetch(`/api/v1/music/audio-quality-profiles/${id}`, {
    method: "DELETE",
  });
  if (!res.ok && res.status !== 204) {
    throw new Error(`delete audio quality profile failed: ${res.status}`);
  }
}

export async function createMetadataProfile(
  req: CreateMetadataProfileRequest,
): Promise<MetadataProfile> {
  const res = await apiFetch("/api/v1/music/metadata-profiles", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
  if (!res.ok) {
    throw new Error(`create metadata profile failed: ${res.status}`);
  }
  return (await res.json()) as MetadataProfile;
}

export async function updateMetadataProfile(
  id: string,
  req: UpdateMetadataProfileRequest,
): Promise<MetadataProfile> {
  const res = await apiFetch(`/api/v1/music/metadata-profiles/${id}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
  if (!res.ok) {
    throw new Error(`update metadata profile failed: ${res.status}`);
  }
  return (await res.json()) as MetadataProfile;
}

export async function deleteMetadataProfile(id: string): Promise<void> {
  const res = await apiFetch(`/api/v1/music/metadata-profiles/${id}`, {
    method: "DELETE",
  });
  if (!res.ok && res.status !== 204) {
    throw new Error(`delete metadata profile failed: ${res.status}`);
  }
}

export async function searchAlbum(id: string): Promise<AlbumGrabResult> {
  const res = await apiFetch(`/api/v1/albums/${id}/search`, { method: "POST" });
  if (!res.ok) {
    let msg = `${res.status}`;
    try {
      const body = (await res.json()) as { error?: string };
      if (body.error) msg = body.error;
    } catch {
      /* ignore */
    }
    throw new Error(msg);
  }
  return (await res.json()) as AlbumGrabResult;
}

export async function searchAlbumUpgrade(id: string): Promise<AlbumGrabResult> {
  const res = await apiFetch(`/api/v1/albums/${id}/search/upgrade`, {
    method: "POST",
  });
  if (!res.ok) {
    let msg = `${res.status}`;
    try {
      const body = (await res.json()) as { error?: string };
      if (body.error) msg = body.error;
    } catch {
      /* ignore */
    }
    throw new Error(msg);
  }
  return (await res.json()) as AlbumGrabResult;
}

export async function fetchAlbumReleases(
  id: string,
  signal?: AbortSignal,
): Promise<ReleaseCandidate[]> {
  return getJSON<ReleaseCandidate[]>(`/api/v1/albums/${id}/releases`, signal);
}

export async function grabAlbumRelease(
  id: string,
  release: ReleaseCandidate,
): Promise<AlbumGrabResult> {
  const res = await apiFetch(`/api/v1/albums/${id}/grab`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      title: release.title,
      indexer_id: release.indexer_id,
      link: release.link,
      magnet_uri: release.magnet_uri,
      infohash: release.infohash,
      size: release.size,
      seeders: release.seeders,
    }),
  });
  if (!res.ok) {
    let msg = `${res.status}`;
    try {
      const body = (await res.json()) as { error?: string };
      if (body.error) msg = body.error;
    } catch {
      /* ignore */
    }
    throw new Error(msg);
  }
  return (await res.json()) as AlbumGrabResult;
}

// ---------- Hooks ----------

export function useArtists() {
  return useQuery({
    queryKey: musicKeys.artists,
    queryFn: ({ signal }) => fetchArtists(signal),
    staleTime: 15_000,
  });
}

export function useArtist(id: string | undefined) {
  return useQuery({
    queryKey: musicKeys.artist(id ?? ""),
    queryFn: ({ signal }) => fetchArtist(id as string, signal),
    enabled: !!id,
  });
}

export function useAlbum(id: string | undefined) {
  return useQuery({
    queryKey: musicKeys.album(id ?? ""),
    queryFn: ({ signal }) => fetchAlbum(id as string, signal),
    enabled: !!id,
  });
}

export function useAudioQualityDefinitions() {
  return useQuery({
    queryKey: musicKeys.audioQualityDefinitions,
    queryFn: ({ signal }) =>
      getJSON<AudioQualityDefinition[]>(
        "/api/v1/music/audio-quality-definitions",
        signal,
      ),
    staleTime: 60_000,
  });
}

export function useAudioQualityProfiles() {
  return useQuery({
    queryKey: musicKeys.audioQualityProfiles,
    queryFn: ({ signal }) =>
      getJSON<AudioQualityProfile[]>(
        "/api/v1/music/audio-quality-profiles",
        signal,
      ),
    staleTime: 60_000,
  });
}

export function useUpdateAudioQualityProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; req: UpdateAudioQualityProfileRequest }) =>
      updateAudioQualityProfile(vars.id, vars.req),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: musicKeys.audioQualityProfiles }),
  });
}

export function useCreateAudioQualityProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: createAudioQualityProfile,
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: musicKeys.audioQualityProfiles }),
  });
}

export function useDeleteAudioQualityProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: deleteAudioQualityProfile,
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: musicKeys.audioQualityProfiles }),
  });
}

async function sendMetadataSource<T>(
  path: string,
  method: "PUT" | "POST",
  req: MetadataSourceUpdate | ArtworkSettingsUpdate,
): Promise<T> {
  const res = await apiFetch(path, {
    method,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
  if (!res.ok) {
    const detail = (await res.text()).trim();
    throw new Error(detail || `metadata source request failed: ${res.status}`);
  }
  return (await res.json()) as T;
}

export function useMetadataSource(enabled = true) {
  return useQuery({
    queryKey: musicKeys.metadataSource,
    queryFn: ({ signal }) =>
      getJSON<MetadataSource>("/api/v1/music/metadata-source", signal),
    enabled,
    staleTime: 60_000,
  });
}

export function useUpdateMetadataSource() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: MetadataSourceUpdate) =>
      sendMetadataSource<MetadataSource>(
        "/api/v1/music/metadata-source",
        "PUT",
        req,
      ),
    onSuccess: (src) => qc.setQueryData(musicKeys.metadataSource, src),
  });
}

export function useTestMetadataSource() {
  return useMutation({
    mutationFn: (req: MetadataSourceUpdate) =>
      sendMetadataSource<MetadataSourceTestResult>(
        "/api/v1/music/metadata-source/test",
        "POST",
        req,
      ),
  });
}

export function useArtworkSettings(enabled = true) {
  return useQuery({
    queryKey: musicKeys.artworkSettings,
    queryFn: ({ signal }) =>
      getJSON<ArtworkSettings>("/api/v1/music/artwork-settings", signal),
    enabled,
    staleTime: 60_000,
  });
}

export function useUpdateArtworkSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: ArtworkSettingsUpdate) =>
      sendMetadataSource<ArtworkSettings>(
        "/api/v1/music/artwork-settings",
        "PUT",
        req,
      ),
    onSuccess: (settings) =>
      qc.setQueryData(musicKeys.artworkSettings, settings),
  });
}

export function useTestArtworkSettings() {
  return useMutation({
    mutationFn: (req: ArtworkSettingsUpdate) =>
      sendMetadataSource<ArtworkTestResult>(
        "/api/v1/music/artwork-settings/test",
        "POST",
        req,
      ),
  });
}

export function useMetadataProfiles() {
  return useQuery({
    queryKey: musicKeys.metadataProfiles,
    queryFn: ({ signal }) =>
      getJSON<MetadataProfile[]>("/api/v1/music/metadata-profiles", signal),
    staleTime: 60_000,
  });
}

export function useCreateMetadataProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: createMetadataProfile,
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: musicKeys.metadataProfiles }),
  });
}

export function useUpdateMetadataProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; req: UpdateMetadataProfileRequest }) =>
      updateMetadataProfile(vars.id, vars.req),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: musicKeys.metadataProfiles }),
  });
}

export function useDeleteMetadataProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: deleteMetadataProfile,
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: musicKeys.metadataProfiles }),
  });
}

export function useAddArtist() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: addArtist,
    onSuccess: () => qc.invalidateQueries({ queryKey: musicKeys.artists }),
  });
}

export function useDeleteArtist() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: deleteArtist,
    onSuccess: () => qc.invalidateQueries({ queryKey: musicKeys.artists }),
  });
}

export function useSetArtistMonitoring() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, status }: { id: string; status: MonitoringStatus }) =>
      setArtistMonitoring(id, status),
    onSuccess: (a) => {
      qc.invalidateQueries({ queryKey: musicKeys.artists });
      qc.invalidateQueries({ queryKey: musicKeys.artist(a.id) });
    },
  });
}

export function useRefreshArtist() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: refreshArtist,
    onSuccess: (a) => {
      qc.setQueryData(musicKeys.artist(a.id), a);
      qc.invalidateQueries({ queryKey: musicKeys.artists });
    },
  });
}

export function useRescanArtist() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, libraryId }: { id: string; libraryId: string }) =>
      rescanArtist(id, libraryId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: musicKeys.artists });
    },
  });
}

export function useSetAlbumMonitored() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, monitored }: { id: string; monitored: boolean }) =>
      setAlbumMonitored(id, monitored),
    onSuccess: (al) => {
      qc.invalidateQueries({ queryKey: musicKeys.artists });
      qc.invalidateQueries({ queryKey: ["music", "artist"] });
      qc.invalidateQueries({ queryKey: musicKeys.albums });
      qc.invalidateQueries({ queryKey: musicKeys.album(al.id) });
    },
  });
}

export function useSetTrackMonitored() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: {
      albumId: string;
      trackId: string;
      monitored: boolean;
      artistId?: string;
    }) => setTrackMonitored(vars.albumId, vars.trackId, vars.monitored),
    onSuccess: (_track, vars) => {
      qc.invalidateQueries({ queryKey: musicKeys.album(vars.albumId) });
      qc.invalidateQueries({ queryKey: musicKeys.albums });
      qc.invalidateQueries({ queryKey: musicKeys.artists });
      qc.invalidateQueries({ queryKey: ["music", "artist"] });
    },
  });
}

export function useUpdateArtist() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { id: string; req: UpdateArtistRequest }) =>
      updateArtist(vars.id, vars.req),
    onSuccess: (artist) => {
      qc.invalidateQueries({ queryKey: musicKeys.artist(artist.id) });
      qc.invalidateQueries({ queryKey: musicKeys.artists });
    },
  });
}

export function useArtistFiles(id: string | undefined, enabled = true) {
  return useQuery({
    queryKey: musicKeys.artistFiles(id ?? ""),
    queryFn: ({ signal }) => fetchArtistFiles(id!, signal),
    enabled: !!id && enabled,
  });
}

export function useArtistHistory(id: string | undefined, enabled = true) {
  return useQuery({
    queryKey: musicKeys.artistHistory(id ?? ""),
    queryFn: ({ signal }) => fetchArtistHistory(id!, signal),
    enabled: !!id && enabled,
  });
}

export function useSelectAlbumRelease() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { albumId: string; releaseId: string }) =>
      selectAlbumRelease(vars.albumId, vars.releaseId),
    onSuccess: (album) => {
      qc.invalidateQueries({ queryKey: musicKeys.album(album.id) });
      qc.invalidateQueries({ queryKey: musicKeys.albums });
      qc.invalidateQueries({ queryKey: musicKeys.artists });
      qc.invalidateQueries({ queryKey: ["music", "artist"] });
    },
  });
}

export function useAlbums() {
  return useQuery({
    queryKey: musicKeys.albums,
    queryFn: ({ signal }) => fetchAlbums(signal),
    staleTime: 15_000,
  });
}

export function useLookupAlbums(query: string) {
  return useQuery({
    queryKey: ["music", "album-lookup", query],
    queryFn: ({ signal }) => lookupAlbums(query, signal),
    enabled: query.trim().length >= 2,
    staleTime: 30_000,
  });
}

export function useAddAlbum() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: addAlbum,
    onSuccess: async (album) => {
      await Promise.all([
        qc.invalidateQueries({ queryKey: musicKeys.albums }),
        qc.invalidateQueries({ queryKey: musicKeys.artists }),
        album.artist_id
          ? qc.invalidateQueries({
              queryKey: musicKeys.artist(album.artist_id),
            })
          : Promise.resolve(),
      ]);
    },
  });
}

export function useSearchAlbum() {
  return useMutation({ mutationFn: (id: string) => searchAlbum(id) });
}

export function useSearchAlbumUpgrade() {
  return useMutation({ mutationFn: (id: string) => searchAlbumUpgrade(id) });
}

export function useAlbumReleases(id: string | undefined, enabled: boolean) {
  return useQuery({
    queryKey: ["music", "album-releases", id ?? ""],
    queryFn: ({ signal }) => fetchAlbumReleases(id ?? "", signal),
    enabled: enabled && !!id,
    staleTime: 60_000,
  });
}

export function useGrabAlbumRelease() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, release }: { id: string; release: ReleaseCandidate }) =>
      grabAlbumRelease(id, release),
    onSuccess: (_res, vars) => {
      qc.invalidateQueries({ queryKey: musicKeys.album(vars.id) });
    },
  });
}
