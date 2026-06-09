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

export interface Album {
  id: string;
  mbid: string;
  artist_id: string;
  title: string;
  album_type?: string;
  secondary_types?: string[];
  release_date?: string;
  genres?: string[];
  cover_art_url?: string;
  overview?: string;
  monitored: boolean;
  selected_release_id?: string;
  last_search_at?: string;
  releases?: AlbumRelease[];
  tracks?: Track[];
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

export interface AudioQualityProfile {
  id: string;
  name: string;
  items: unknown;
  cutoff?: string;
  upgrade_allowed: boolean;
}

export interface MetadataProfile {
  id: string;
  name: string;
  primary_types: string[];
  secondary_types: string[];
  release_statuses: string[];
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

// ---------- Query keys ----------

export const musicKeys = {
  artists: ["music", "artists"] as const,
  artist: (id: string) => ["music", "artist", id] as const,
  album: (id: string) => ["music", "album", id] as const,
  lookup: (q: string) => ["music", "lookup", q] as const,
  audioQualityDefinitions: ["music", "audio-quality-definitions"] as const,
  audioQualityProfiles: ["music", "audio-quality-profiles"] as const,
  metadataProfiles: ["music", "metadata-profiles"] as const,
};

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

export function useMetadataProfiles() {
  return useQuery({
    queryKey: musicKeys.metadataProfiles,
    queryFn: ({ signal }) =>
      getJSON<MetadataProfile[]>("/api/v1/music/metadata-profiles", signal),
    staleTime: 60_000,
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

export function useSetAlbumMonitored() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, monitored }: { id: string; monitored: boolean }) =>
      setAlbumMonitored(id, monitored),
    onSuccess: (al) => {
      qc.invalidateQueries({ queryKey: musicKeys.artist(al.artist_id) });
      qc.invalidateQueries({ queryKey: musicKeys.album(al.id) });
    },
  });
}

export function useSearchAlbum() {
  return useMutation({ mutationFn: (id: string) => searchAlbum(id) });
}
