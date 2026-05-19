import { useEffect, useState } from "react";
import { apiFetch } from "@/lib/fetch";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from "@/components/ui/sheet";
import {
  Loader2, Tv, Star,
  Trash2, Pencil,
  Bookmark, BookmarkCheck, RefreshCw, ChevronRight, ChevronDown,
  Users, FileVideo, Search, FolderSearch,
  Archive, ArchiveRestore, HardDriveDownload,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { toast } from "sonner";
import { SeriesStatusBadge } from "./series-status-badge";
import { ReleaseSearchDialog } from "@/components/search/release-search-dialog";
import { autoSearch } from "@/lib/autosearch-api";
import { AltTitlesSection } from "@/components/alt-titles";
import { PersonDiscoverDialog } from "@/components/person-discover-dialog";
import type { Library } from "../../lib/libraries-api";
import type { Series, Season, Episode, QualityProfile, Credits, CreditPerson } from "./types";
import { TMDB_IMG } from "./types";

// ─── Helpers ──────────────────────────────────────────────────────────

function CollapsibleSection({
  title,
  icon: Icon,
  defaultOpen = true,
  children,
}: {
  title: string;
  icon: React.ComponentType<{ className?: string }>;
  defaultOpen?: boolean;
  children: React.ReactNode;
}) {
  const [open, setOpen] = useState(defaultOpen);
  return (
    <div className="border-t border-border/40">
      <button
        onClick={() => setOpen(v => !v)}
        className="flex items-center gap-2 w-full py-3 text-sm font-semibold text-muted-foreground hover:text-foreground transition-colors"
      >
        <Icon className="w-4 h-4" />
        {title}
        <ChevronRight className={cn("w-4 h-4 ml-auto transition-transform duration-200", open && "rotate-90")} />
      </button>
      {open && <div className="pb-4">{children}</div>}
    </div>
  );
}

function DetailRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <span className="text-muted-foreground text-xs uppercase tracking-wider">{label}</span>
      <div className="mt-1 text-sm">{children}</div>
    </div>
  );
}

const PROFILE_IMG = "https://image.tmdb.org/t/p/w185";

function PersonCard({ person, onClick }: { person: CreditPerson; onClick?: () => void }) {
  return (
    <div
      className={cn("flex-shrink-0 w-[100px] group", onClick && "cursor-pointer")}
      onClick={onClick}
      role={onClick ? "button" : undefined}
      tabIndex={onClick ? 0 : undefined}
      onKeyDown={onClick ? (e) => { if (e.key === "Enter" || e.key === " ") onClick(); } : undefined}
    >
      <div className="relative w-[100px] h-[150px] rounded-lg overflow-hidden bg-muted/30 mb-1.5">
        {person.profile_path ? (
          <img src={`${PROFILE_IMG}${person.profile_path}`} alt={person.name} className="w-full h-full object-cover" loading="lazy" />
        ) : (
          <div className="w-full h-full flex items-center justify-center text-muted-foreground/30">
            <Users className="w-8 h-8" />
          </div>
        )}
      </div>
      <p className="text-xs font-medium truncate" title={person.name}>{person.name}</p>
      {(person.character || person.role) && (
        <p className="text-[11px] text-muted-foreground truncate" title={person.character || person.role}>
          {person.character || person.role}
        </p>
      )}
    </div>
  );
}

// ─── Episode Status (Sonarr-style) ────────────────────────────────────

type EpisodeStatus = "downloaded" | "grabbed" | "missing" | "unaired" | "unmonitored";

function getEpisodeStatus(ep: Episode): EpisodeStatus {
  if (!ep.monitored) return "unmonitored";
  if (ep.hasFile) return "downloaded";
  if (ep.grabbed) return "grabbed";
  const today = new Date().toISOString().slice(0, 10);
  if (!ep.airDate || ep.airDate > today) return "unaired";
  return "missing";
}

const EPISODE_STATUS_STYLE: Record<EpisodeStatus, { bg: string; label: string; text: string }> = {
  downloaded:  { bg: "bg-green-600",   label: "Downloaded", text: "text-white" },
  grabbed:     { bg: "bg-amber-600",   label: "Grabbed",    text: "text-white" },
  missing:     { bg: "bg-red-600",     label: "Missing",    text: "text-white" },
  unaired:     { bg: "bg-blue-600",    label: "Unaired",    text: "text-white" },
  unmonitored: { bg: "bg-zinc-600",    label: "Unmonitored", text: "text-zinc-300" },
};

function EpisodeStatusBadge({ status }: { status: EpisodeStatus }) {
  const s = EPISODE_STATUS_STYLE[status];
  return (
    <span className={cn("text-[10px] font-semibold px-2 py-0.5 rounded-sm whitespace-nowrap", s.bg, s.text)}>
      {s.label}
    </span>
  );
}

function formatAirDate(dateStr: string): string {
  try {
    const d = new Date(dateStr + "T00:00:00");
    return d.toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" });
  } catch {
    return dateStr;
  }
}

function getSeasonStats(episodes: Episode[]) {
  let downloaded = 0, grabbed = 0, missing = 0, unaired = 0, unmonitored = 0;
  const today = new Date().toISOString().slice(0, 10);
  for (const ep of episodes) {
    if (!ep.monitored) { unmonitored++; continue; }
    if (ep.hasFile) { downloaded++; continue; }
    if (ep.grabbed) { grabbed++; continue; }
    if (!ep.airDate || ep.airDate > today) { unaired++; continue; }
    missing++;
  }
  const aired = downloaded + grabbed + missing;
  const percent = aired > 0 ? Math.round((downloaded / aired) * 100) : (episodes.length > 0 ? 100 : 0);
  return { downloaded, grabbed, missing, unaired, unmonitored, aired, total: episodes.length, percent };
}

// ─── Season/Episode Accordion ─────────────────────────────────────────

function SeasonAccordion({
  seriesId,
  season,
  seriesTitle,
  seriesData,
  onSearchOpen,
}: {
  seriesId: string;
  season: Season;
  seriesTitle: string;
  seriesData: { qualityProfileId: string; imdbId?: string; tmdbId?: string; tvdbId?: string };
  onSearchOpen: (ctx: {
    title: string;
    query: string;
    season?: number;
    episode?: number;
    mediaType: "movie" | "episode" | "season" | "series";
    seriesId?: string;
    episodeIds?: string[];
  }) => void;
}) {
  const [open, setOpen] = useState(false);
  const [episodes, setEpisodes] = useState<Episode[]>([]);
  const [loading, setLoading] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [autoSearchingEp, setAutoSearchingEp] = useState<number | null>(null);
  const [autoSearchingSeason, setAutoSearchingSeason] = useState(false);

  const fetchEpisodes = async () => {
    try {
      const res = await apiFetch(`/api/v1/series/${seriesId}/seasons/${season.seasonNumber}/episodes`);
      if (res.ok) {
        const data = await res.json();
        setEpisodes(Array.isArray(data) ? data : Array.isArray(data?.data) ? data.data : []);
      }
    } catch { /* ignore */ }
  };

  const handleToggle = async () => {
    if (!open && !loaded) {
      setLoading(true);
      await fetchEpisodes();
      setLoading(false);
      setLoaded(true);
    }
    setOpen(v => !v);
  };

  const handleAutoSearchEpisode = async (ep: Episode) => {
    setAutoSearchingEp(ep.episodeNumber);
    const toastId = toast.loading(`Searching for ${seriesTitle} S${String(season.seasonNumber).padStart(2, "0")}E${String(ep.episodeNumber).padStart(2, "0")}...`);
    try {
      const result = await autoSearch({
        media_type: "series",
        media_id: seriesId,
        title: seriesTitle,
        quality_profile_id: seriesData.qualityProfileId,
        imdb_id: seriesData.imdbId || undefined,
        tmdb_id: seriesData.tmdbId || undefined,
        tvdb_id: seriesData.tvdbId || undefined,
        season: season.seasonNumber,
        episode: ep.episodeNumber,
      });
      if (result.grabbed) {
        toast.success(`Grabbed: ${result.grabbed.title}`, { id: toastId });
        await fetchEpisodes();
      } else {
        toast.warning(`No suitable release found`, { id: toastId, description: result.reason || `${result.considered} considered, ${result.rejected} rejected` });
      }
    } catch (err: any) {
      toast.error("Auto search failed", { id: toastId, description: err.message });
    } finally {
      setAutoSearchingEp(null);
    }
  };

  const handleAutoSearchSeason = async () => {
    setAutoSearchingSeason(true);
    const label = season.seasonNumber === 0 ? "Specials" : `Season ${season.seasonNumber}`;
    const toastId = toast.loading(`Searching for ${seriesTitle} ${label}...`);
    try {
      const result = await autoSearch({
        media_type: "series",
        media_id: seriesId,
        title: seriesTitle,
        quality_profile_id: seriesData.qualityProfileId,
        imdb_id: seriesData.imdbId || undefined,
        tmdb_id: seriesData.tmdbId || undefined,
        tvdb_id: seriesData.tvdbId || undefined,
        season: season.seasonNumber,
      });
      if (result.grabbed) {
        toast.success(`Grabbed: ${result.grabbed.title}`, { id: toastId });
        if (loaded) await fetchEpisodes();
      } else {
        toast.warning(`No suitable release found`, { id: toastId, description: result.reason || `${result.considered} considered, ${result.rejected} rejected` });
      }
    } catch (err: any) {
      toast.error("Auto search failed", { id: toastId, description: err.message });
    } finally {
      setAutoSearchingSeason(false);
    }
  };

  // Use backend stats initially, switch to computed stats once episodes are loaded
  const computedStats = loaded ? getSeasonStats(episodes) : null;
  const stats = computedStats ?? (season.episodeStats ? {
    downloaded: season.episodeStats.downloadedEpisodes,
    missing: season.episodeStats.missingEpisodes,
    unaired: season.episodeStats.totalEpisodes - season.episodeStats.airedEpisodes,
    unmonitored: season.episodeStats.totalEpisodes - season.episodeStats.monitoredEpisodes,
    aired: season.episodeStats.airedEpisodes,
    total: season.episodeStats.totalEpisodes,
    percent: season.episodeStats.airedEpisodes > 0
      ? Math.round((season.episodeStats.downloadedEpisodes / season.episodeStats.airedEpisodes) * 100)
      : (season.episodeStats.totalEpisodes > 0 ? 100 : 0),
  } : null);

  const badgeColor = !stats ? "bg-zinc-600"
    : stats.percent === 100 ? "bg-green-600"
    : stats.percent > 0 ? "bg-amber-500"
    : stats.aired > 0 ? "bg-red-600"
    : "bg-blue-600";

  return (
    <div className="border border-border/40 rounded-lg overflow-hidden">
      {/* Season header */}
      <button
        onClick={handleToggle}
        className="flex items-center gap-3 w-full px-4 py-3 text-sm hover:bg-accent/5 transition-colors bg-muted/10"
      >
        {open ? <ChevronDown className="w-4 h-4 shrink-0" /> : <ChevronRight className="w-4 h-4 shrink-0" />}

        {/* Season icon + title */}
        <FileVideo className="w-4 h-4 text-muted-foreground shrink-0" />
        <span className="font-semibold text-sm">
          {season.seasonNumber === 0 ? "Specials" : `Season ${season.seasonNumber}`}
        </span>

        {/* Episode count badge (Sonarr-style: downloaded/aired) */}
        {stats && (
          <span className={cn("text-[11px] font-bold px-2 py-0.5 rounded-sm text-white tabular-nums", badgeColor)}>
            {stats.downloaded} / {stats.aired}
          </span>
        )}

        {/* Spacer */}
        <div className="flex-1" />

        {/* Season action icons */}
        <div className="flex items-center gap-1 shrink-0" onClick={e => e.stopPropagation()}>
          <button
            className="p-1 rounded hover:bg-accent/10 text-muted-foreground hover:text-accent transition-colors"
            title="Automatic search for season"
            disabled={autoSearchingSeason}
            onClick={handleAutoSearchSeason}
          >
            {autoSearchingSeason ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Search className="w-3.5 h-3.5" />}
          </button>
          <button
            className="p-1 rounded hover:bg-accent/10 text-muted-foreground hover:text-accent transition-colors"
            title="Interactive search for season"
            onClick={() => onSearchOpen({
              title: `${seriesTitle} ${season.seasonNumber === 0 ? "Specials" : `S${season.seasonNumber.toString().padStart(2, "0")}`}`,
              query: `${seriesTitle} S${season.seasonNumber.toString().padStart(2, "0")}`,
              season: season.seasonNumber,
              mediaType: "season",
              seriesId,
            })}
          >
            <FolderSearch className="w-3.5 h-3.5" />
          </button>
        </div>

        {/* Monitoring toggle */}
        {season.monitored ? (
          <Bookmark className="w-4 h-4 text-accent shrink-0" />
        ) : (
          <Bookmark className="w-4 h-4 text-muted-foreground/30 shrink-0" />
        )}
      </button>

      {/* Episode table */}
      {open && (
        <div className="border-t border-border/30">
          {loading ? (
            <div className="flex items-center justify-center py-6">
              <Loader2 className="w-5 h-5 animate-spin text-muted-foreground" />
            </div>
          ) : episodes.length === 0 ? (
            <div className="text-center py-8 space-y-2">
              {season.episodeCount === 0 ? (
                <>
                  <div className="flex items-center justify-center gap-2 text-indigo-400">
                    <Tv className="w-5 h-5" />
                    <span className="text-sm font-medium">Coming Soon</span>
                  </div>
                  <p className="text-xs text-muted-foreground">No episodes have been announced yet</p>
                </>
              ) : (
                <p className="text-sm text-muted-foreground">No episodes found</p>
              )}
            </div>
          ) : (
            <div>
              {/* Table header */}
              <div className="flex items-center gap-3 px-4 py-2 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground border-b border-border/30 bg-muted/5">
                <span className="w-6 shrink-0" />
                <span className="w-10 text-right shrink-0">#</span>
                <span className="flex-1">Title</span>
                <span className="w-28 text-right shrink-0">Air Date</span>
                <span className="w-24 text-right shrink-0">Status</span>
                <span className="w-16 text-right shrink-0">Actions</span>
              </div>
              {/* Episode rows - newest first like Sonarr */}
              <div className="divide-y divide-border/15">
                {[...episodes].sort((a, b) => b.episodeNumber - a.episodeNumber).map(ep => {
                  const status = getEpisodeStatus(ep);
                  return (
                    <div key={ep.id} className="flex items-center gap-3 px-4 py-2 text-sm hover:bg-accent/5 transition-colors group">
                      {/* Monitored bookmark */}
                      <span className="w-6 shrink-0 flex justify-center">
                        {ep.monitored ? (
                          <Bookmark className="w-3.5 h-3.5 text-accent" />
                        ) : (
                          <Bookmark className="w-3.5 h-3.5 text-muted-foreground/25" />
                        )}
                      </span>

                      {/* Episode number */}
                      <span className="w-10 text-right shrink-0 text-muted-foreground tabular-nums text-xs">
                        {ep.episodeNumber}
                      </span>

                      {/* Title */}
                      <div className="flex-1 min-w-0">
                        <p className="truncate">{ep.title || `Episode ${ep.episodeNumber}`}</p>
                      </div>

                      {/* Air date */}
                      <span className="w-28 text-right shrink-0 text-xs text-muted-foreground tabular-nums">
                        {ep.airDate ? formatAirDate(ep.airDate) : "—"}
                      </span>

                      {/* Status badge */}
                      <span className="w-24 flex justify-end shrink-0">
                        <EpisodeStatusBadge status={status} />
                      </span>

                      {/* Action icons */}
                      <div className="w-16 flex justify-end items-center gap-0.5 shrink-0">
                        <button
                          className="p-1 rounded hover:bg-accent/10 text-muted-foreground hover:text-accent transition-colors"
                          title="Automatic search"
                          disabled={autoSearchingEp === ep.episodeNumber}
                          onClick={() => handleAutoSearchEpisode(ep)}
                        >
                          {autoSearchingEp === ep.episodeNumber ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Search className="w-3.5 h-3.5" />}
                        </button>
                        <button
                          className="p-1 rounded hover:bg-accent/10 text-muted-foreground hover:text-accent transition-colors"
                          title="Interactive search"
                          onClick={() => onSearchOpen({
                            title: `${seriesTitle} S${season.seasonNumber.toString().padStart(2, "0")}E${ep.episodeNumber.toString().padStart(2, "0")}`,
                            query: `${seriesTitle} S${season.seasonNumber.toString().padStart(2, "0")}E${ep.episodeNumber.toString().padStart(2, "0")}`,
                            season: season.seasonNumber,
                            episode: ep.episodeNumber,
                            mediaType: "episode",
                            seriesId,
                            episodeIds: [ep.id],
                          })}
                        >
                          <FolderSearch className="w-3.5 h-3.5" />
                        </button>
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// ─── Series Detail Sheet ──────────────────────────────────────────────

export function SeriesDetailSheet({
  series,
  open,
  onOpenChange,
  profiles,
  libraries,
  onUpdated,
  onDeleted,
  onRefresh,
  existingMovieIds = new Set(),
  existingSeriesIds = new Set(),
}: {
  series: Series | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  profiles: QualityProfile[];
  libraries: Library[];
  onUpdated: (updated: Series) => void;
  onDeleted: (id: string) => void;
  onRefresh?: () => void;
  existingMovieIds?: Set<number>;
  existingSeriesIds?: Set<number>;
}) {
  const [editing, setEditing] = useState(false);
  const [editProfile, setEditProfile] = useState("");
  const [editMonitoring, setEditMonitoring] = useState("");
  const [saving, setSaving] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [overviewExpanded, setOverviewExpanded] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [credits, setCredits] = useState<Credits | null>(null);
  const [creditsLoading, setCreditsLoading] = useState(false);
  const [seasons, setSeasons] = useState<Season[]>([]);
  const [seasonsLoading, setSeasonsLoading] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const [searchContext, setSearchContext] = useState<{
    title: string;
    query: string;
    season?: number;
    episode?: number;
    mediaType: "movie" | "episode" | "season" | "series";
    seriesId?: string;
    episodeIds?: string[];
    movieId?: string;
  } | null>(null);
  const [archiving, setArchiving] = useState(false);
  const [autoSearching, setAutoSearching] = useState(false);
  const [rescanning, setRescanning] = useState(false);
  const [discoverPerson, setDiscoverPerson] = useState<{ id: number; name: string } | null>(null);

  const handleAutoSearch = async () => {
    if (!series) return;
    setAutoSearching(true);
    const toastId = toast.loading(`Searching for ${series.title}...`);
    try {
      const result = await autoSearch({
        media_type: "series",
        media_id: series.id,
        title: series.title,
        quality_profile_id: series.qualityProfileId,
        imdb_id: series.imdbId || undefined,
        tmdb_id: series.tmdbId || undefined,
        tvdb_id: series.tvdbId || undefined,
      });
      if (result.grabbed) {
        toast.success(`Grabbed: ${result.grabbed.title}`, {
          id: toastId,
          description: `Quality tier ${result.grabbed.quality_tier} · Format score ${result.grabbed.format_score} · ${result.considered} considered, ${result.rejected} rejected`,
        });
      } else {
        toast.warning(`No suitable release found`, {
          id: toastId,
          description: result.reason || `${result.considered} considered, ${result.rejected} rejected`,
        });
      }
    } catch (err: any) {
      toast.error("Auto search failed", {
        id: toastId,
        description: err.message,
      });
    } finally {
      setAutoSearching(false);
    }
  };

  const openSearch = (ctx: typeof searchContext) => {
    setSearchContext(ctx);
    setSearchOpen(true);
  };

  useEffect(() => {
    if (series && open) {
      setEditing(false);
      setEditProfile(series.qualityProfileId);
      setEditMonitoring(series.monitoringStatus);
      setOverviewExpanded(false);

      // Fetch credits
      setCredits(null);
      setCreditsLoading(true);
      apiFetch(`/api/v1/series/${series.id}/credits`)
        .then(r => r.ok ? r.json() : null)
        .then(data => setCredits(data))
        .catch((err) => console.error("fetch failed:", err))
        .finally(() => setCreditsLoading(false));

      // Fetch seasons
      setSeasons([]);
      setSeasonsLoading(true);
      apiFetch(`/api/v1/series/${series.id}/seasons`)
        .then(r => r.ok ? r.json() : [])
        .then(data => setSeasons(Array.isArray(data) ? data : Array.isArray(data?.data) ? data.data : []))
        .catch((err) => console.error("fetch failed:", err))
        .finally(() => setSeasonsLoading(false));
    }
  }, [series, open]);

  if (!series) return null;

  const profile = profiles.find(p => p.id === series.qualityProfileId);
  const library = libraries.find(l => l.id === series.libraryId);
  const isMonitored = series.monitoringStatus === "monitored";

  const handleSave = async () => {
    setSaving(true);
    try {
      const res = await apiFetch(`/api/v1/series/${series.id}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          qualityProfileId: editProfile,
          monitoringStatus: editMonitoring,
        }),
      });
      if (res.ok) {
        const updated = await res.json();
        onUpdated(updated);
        setEditing(false);
        toast.success("Series updated");
      } else {
        toast.error("Failed to update series");
      }
    } catch {
      toast.error("Network error");
    } finally {
      setSaving(false);
    }
  };

  const handleToggleMonitoring = async () => {
    const newStatus = isMonitored ? "unmonitored" : "monitored";
    try {
      const res = await apiFetch(`/api/v1/series/${series.id}/monitoring`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ status: newStatus }),
      });
      if (res.ok) {
        onUpdated({ ...series, monitoringStatus: newStatus });
        toast.success(newStatus === "monitored" ? "Now monitoring" : "Unmonitored");
      }
    } catch {
      toast.error("Failed to update monitoring");
    }
  };

  const handleDelete = async () => {
    setDeleting(true);
    try {
      const res = await apiFetch(`/api/v1/series/${series.id}`, {
        method: "DELETE",
      });
      if (res.ok || res.status === 204) {
        onDeleted(series.id);
        onOpenChange(false);
        setDeleteOpen(false);
        toast.success("Series deleted");
      } else {
        toast.error("Failed to delete series");
      }
    } catch {
      toast.error("Network error");
    } finally {
      setDeleting(false);
    }
  };

  const handleArchiveToggle = async () => {
    if (!series) return;
    setArchiving(true);
    const isArchived = series.monitoringStatus === "unmonitored";
    const endpoint = isArchived
      ? `/api/v1/series/${series.id}/unarchive`
      : `/api/v1/series/${series.id}/archive`;
    try {
      const res = await apiFetch(endpoint, { method: "POST" });
      if (res.ok) {
        const newStatus = isArchived ? "monitored" : "unmonitored";
        onUpdated({ ...series, monitoringStatus: newStatus });
        toast.success(isArchived ? "Series unarchived" : "Series archived");
      } else {
        toast.error("Failed to update archive status");
      }
    } catch {
      toast.error("Network error");
    } finally {
      setArchiving(false);
    }
  };

  const handleRefresh = async () => {
    setRefreshing(true);
    try {
      const refreshRes = await apiFetch(`/api/v1/series/${series.id}/refresh`, { method: "POST" });
      if (!refreshRes.ok) {
        toast.error("Failed to refresh");
        return;
      }
      const res = await apiFetch(`/api/v1/series/${series.id}`);
      if (res.ok) {
        const updated = await res.json();
        onUpdated(updated);
        toast.success("Series refreshed");
      } else {
        toast.error("Failed to fetch updated series");
      }
    } catch {
      toast.error("Network error");
    } finally {
      setRefreshing(false);
    }
  };

  const handleRescan = async () => {
    if (!series.libraryId) {
      toast.error("Series has no library assigned");
      return;
    }
    setRescanning(true);
    try {
      const res = await apiFetch(`/api/v1/series/${series.id}/rescan`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ libraryId: series.libraryId }),
      });
      if (res.ok) {
        const result = await res.json();
        toast.success(`Rescan complete: ${result.matched} matched, ${result.imported} imported`);
        // Refresh series data to reflect any new files
        const updated = await apiFetch(`/api/v1/series/${series.id}`);
        if (updated.ok) onUpdated(await updated.json());
      } else {
        toast.error("Rescan failed");
      }
    } catch {
      toast.error("Network error");
    } finally {
      setRescanning(false);
    }
  };

  const overviewIsLong = (series.overview?.length ?? 0) > 280;

  return (
    <>
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent side="right" className="w-full sm:max-w-2xl p-0 overflow-y-auto">
          <SheetHeader className="sr-only">
            <SheetTitle>{series.title}</SheetTitle>
            <SheetDescription>Series details for {series.title}</SheetDescription>
          </SheetHeader>

          {/* Backdrop header */}
          <div className="relative h-[300px] bg-muted overflow-hidden">
            {series.backdropPath ? (
              <img src={`${TMDB_IMG}/w780${series.backdropPath}`} alt="" className="w-full h-full object-cover" />
            ) : series.posterPath ? (
              <img src={`${TMDB_IMG}/w780${series.posterPath}`} alt="" className="w-full h-full object-cover opacity-30 blur-md scale-110" />
            ) : null}
            <div className="absolute inset-0 bg-gradient-to-t from-background via-background/70 to-black/30" />
            <div className="absolute inset-0 bg-gradient-to-r from-background/80 via-transparent to-transparent" />

            {/* Poster */}
            <div className="absolute bottom-[-40px] left-6 z-20">
              <div className="w-[130px] rounded-lg overflow-hidden shadow-2xl border-4 border-background">
                {series.posterPath ? (
                  <img src={`${TMDB_IMG}/w300${series.posterPath}`} alt={series.title} className="w-full aspect-[2/3] object-cover" />
                ) : (
                  <div className="w-full aspect-[2/3] bg-muted flex items-center justify-center">
                    <Tv className="w-10 h-10 text-muted-foreground/30" />
                  </div>
                )}
              </div>
            </div>

            {/* Title */}
            <div className="absolute bottom-4 left-[170px] right-6 z-10">
              <h2 className="text-2xl font-bold text-white truncate drop-shadow-lg">{series.title}</h2>
              <p className="text-sm text-white/70 mt-0.5 drop-shadow">
                {series.year > 0 && series.year}
                {series.network && ` • ${series.network}`}
              </p>
            </div>

            {/* Monitoring toggle — offset left to avoid sheet close button */}
            <button
              onClick={handleToggleMonitoring}
              className={cn(
                "absolute top-4 right-14 z-20 p-2 rounded-full transition-all duration-200 shadow-lg",
                isMonitored
                  ? "bg-accent text-accent-foreground hover:bg-accent/90"
                  : "bg-black/50 text-white/70 hover:bg-black/70 hover:text-white",
              )}
              title={isMonitored ? "Monitored — click to unmonitor" : "Unmonitored — click to monitor"}
            >
              {isMonitored ? <BookmarkCheck className="w-5 h-5" /> : <Bookmark className="w-5 h-5" />}
            </button>
          </div>

          {/* Spacer for poster overlap */}
          <div className="h-12" />

          {/* Action buttons */}
          <div className="px-6 pb-2">
            <div className="flex items-center gap-1.5">
              {/* Primary actions with labels */}
              <Button size="sm" variant="outline" className="gap-1.5 h-8 text-xs" title="Automatic search for all episodes" onClick={handleAutoSearch} disabled={autoSearching}>
                {autoSearching ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Search className="w-3.5 h-3.5" />}{autoSearching ? "Searching..." : "Search"}
              </Button>
              <Button size="sm" variant="outline" className="gap-1.5 h-8 text-xs" title="Interactive search — browse releases manually" onClick={() => openSearch({ title: series.title, query: series.title, mediaType: "series" })}>
                <FolderSearch className="w-3.5 h-3.5" />Browse
              </Button>
              <Button size="sm" variant="outline" className="gap-1.5 h-8 text-xs" onClick={() => { setEditing(true); setEditProfile(series.qualityProfileId); setEditMonitoring(series.monitoringStatus); }} title="Edit series settings">
                <Pencil className="w-3.5 h-3.5" />Edit
              </Button>

              {/* Separator */}
              <div className="w-px h-5 bg-border mx-0.5" />

              {/* Secondary actions — icon only */}
              <Button size="icon" variant="ghost" className="h-8 w-8" onClick={handleRefresh} disabled={refreshing} title="Refresh metadata from TMDB">
                {refreshing ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <RefreshCw className="w-3.5 h-3.5" />}
              </Button>
              <Button size="icon" variant="ghost" className="h-8 w-8" onClick={handleRescan} disabled={rescanning} title="Rescan library folder for new files">
                {rescanning ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <HardDriveDownload className="w-3.5 h-3.5" />}
              </Button>
              <Button size="icon" variant="ghost" className="h-8 w-8" onClick={handleArchiveToggle} disabled={archiving} title={series.monitoringStatus === "unmonitored" ? "Unarchive" : "Archive"}>
                {series.monitoringStatus === "unmonitored" ? <ArchiveRestore className="w-3.5 h-3.5" /> : <Archive className="w-3.5 h-3.5" />}
              </Button>

              {/* Delete — pushed right */}
              <Button size="icon" variant="ghost" className="h-8 w-8 ml-auto text-destructive hover:text-destructive hover:bg-destructive/10" onClick={() => setDeleteOpen(true)} title="Delete series">
                <Trash2 className="w-3.5 h-3.5" />
              </Button>
            </div>
          </div>

          {/* Edit mode bar */}
          {editing && (
            <div className="mx-6 mb-3 p-4 rounded-lg border border-accent/30 bg-accent/5 space-y-4">
              <div className="flex items-center gap-2 text-sm font-semibold text-accent">
                <Pencil className="w-4 h-4" />Editing Series
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-1.5">
                  <label className="text-xs font-medium">Quality Profile</label>
                  <Select value={editProfile} onValueChange={setEditProfile}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>{profiles.map(p => <SelectItem key={p.id} value={p.id}>{p.name}</SelectItem>)}</SelectContent>
                  </Select>
                </div>
                <div className="space-y-1.5">
                  <label className="text-xs font-medium">Monitoring</label>
                  <Select value={editMonitoring} onValueChange={setEditMonitoring}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="monitored">Monitored</SelectItem>
                      <SelectItem value="unmonitored">Unmonitored</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>
              <div className="flex justify-end gap-2">
                <Button size="sm" variant="outline" onClick={() => setEditing(false)}>Cancel</Button>
                <Button size="sm" onClick={handleSave} disabled={saving}>
                  {saving ? <Loader2 className="w-4 h-4 animate-spin" /> : "Save"}
                </Button>
              </div>
            </div>
          )}

          {/* Content */}
          <div className="px-6 pb-6 space-y-1">
            {/* Overview */}
            <CollapsibleSection title="Overview" icon={Tv}>
              <div className="grid grid-cols-2 gap-4 mb-4">
                <DetailRow label="Status"><SeriesStatusBadge status={series.status} /></DetailRow>
                <DetailRow label="Network">{series.network || "—"}</DetailRow>
                {series.rating > 0 && (
                  <DetailRow label="Rating">
                    <span className="flex items-center gap-1">
                      <Star className="w-3.5 h-3.5 text-yellow-400 fill-yellow-400" />
                      {series.rating.toFixed(1)}
                    </span>
                  </DetailRow>
                )}
                {series.runtime > 0 && (
                  <DetailRow label="Runtime">{series.runtime} min</DetailRow>
                )}
                <DetailRow label="Type"><span className="capitalize">{series.seriesType || "Standard"}</span></DetailRow>
                {profile && <DetailRow label="Quality">{profile.name}</DetailRow>}
                {library && <DetailRow label="Library">{library.name}</DetailRow>}
              </div>
              {series.genres?.length > 0 && (
                <div className="flex gap-1.5 flex-wrap mb-3">
                  {series.genres.map(g => <Badge key={g} variant="secondary" className="text-[10px]">{g}</Badge>)}
                </div>
              )}
              {series.overview && (
                <div>
                  <p className={cn("text-sm text-muted-foreground leading-relaxed", !overviewExpanded && overviewIsLong && "line-clamp-4")}>
                    {series.overview}
                  </p>
                  {overviewIsLong && (
                    <button onClick={() => setOverviewExpanded(v => !v)} className="text-xs text-accent mt-1 hover:underline">
                      {overviewExpanded ? "Show less" : "Read more"}
                    </button>
                  )}
                </div>
              )}
            </CollapsibleSection>

            {/* Alt Titles */}
            <AltTitlesSection mediaId={series.id} mediaType="series" />

            {/* Seasons */}
            <CollapsibleSection title={`Seasons (${seasons.length})`} icon={FileVideo}>
              {seasonsLoading ? (
                <div className="flex items-center justify-center py-6">
                  <Loader2 className="w-5 h-5 animate-spin text-muted-foreground" />
                </div>
              ) : seasons.length === 0 ? (
                <p className="text-sm text-muted-foreground text-center py-4">No seasons found</p>
              ) : (
                <div className="space-y-2">
                  {seasons
                    .sort((a, b) => a.seasonNumber - b.seasonNumber)
                    .map(season => (
                      <SeasonAccordion key={season.id} seriesId={series.id} season={season} seriesTitle={series.title} seriesData={{ qualityProfileId: series.qualityProfileId, imdbId: series.imdbId, tmdbId: series.tmdbId, tvdbId: series.tvdbId }} onSearchOpen={openSearch} />
                    ))}
                </div>
              )}
            </CollapsibleSection>

            {/* Cast & Crew */}
            <CollapsibleSection title="Cast & Crew" icon={Users} defaultOpen={false}>
              {creditsLoading ? (
                <div className="flex items-center justify-center py-6">
                  <Loader2 className="w-5 h-5 animate-spin text-muted-foreground" />
                </div>
              ) : !credits ? (
                <p className="text-sm text-muted-foreground text-center py-4">No credits available</p>
              ) : (
                <div className="space-y-4">
                  {credits.cast?.length > 0 && (
                    <div>
                      <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">Cast</h4>
                      <div className="flex gap-3 overflow-x-auto pb-2">
                        {credits.cast.slice(0, 20).map(p => <PersonCard key={p.id} person={p} onClick={() => setDiscoverPerson({ id: p.id, name: p.name })} />)}
                      </div>
                    </div>
                  )}
                  {credits.crew?.length > 0 && (
                    <div>
                      <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">Crew</h4>
                      <div className="flex gap-3 overflow-x-auto pb-2">
                        {credits.crew.slice(0, 10).map(p => <PersonCard key={`${p.id}-${p.department}`} person={p} onClick={() => setDiscoverPerson({ id: p.id, name: p.name })} />)}
                      </div>
                    </div>
                  )}
                </div>
              )}
            </CollapsibleSection>
          </div>
        </SheetContent>
      </Sheet>

      {/* Delete confirmation */}
      <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>Delete Series</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete &ldquo;{series.title}&rdquo;? This cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <div className="flex justify-end gap-2 mt-4">
            <Button variant="outline" onClick={() => setDeleteOpen(false)}>Cancel</Button>
            <Button variant="destructive" disabled={deleting} onClick={handleDelete}>
              {deleting ? <Loader2 className="w-4 h-4 animate-spin" /> : "Delete"}
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      {/* Release search dialog */}
      {searchContext && (
        <ReleaseSearchDialog
          open={searchOpen}
          onOpenChange={setSearchOpen}
          title={searchContext.title}
          query={searchContext.query}
          tmdbId={series.tmdbId ? Number(series.tmdbId) : undefined}
          imdbId={series.imdbId}
          season={searchContext.season}
          episode={searchContext.episode}
          mediaType={searchContext.mediaType}
          seriesId={searchContext.seriesId}
          episodeIds={searchContext.episodeIds}
          autoSearch
        />
      )}

      {/* Person discover dialog */}
      {discoverPerson && (
        <PersonDiscoverDialog
          open={!!discoverPerson}
          onOpenChange={(o) => { if (!o) setDiscoverPerson(null); }}
          personId={discoverPerson.id}
          personName={discoverPerson.name}
          libraries={libraries}
          qualityProfiles={profiles}
          existingMovieIds={existingMovieIds}
          existingSeriesIds={existingSeriesIds}
          onAdded={onRefresh}
        />
      )}
    </>
  );
}
