import { useEffect, useState } from "react";
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
  Loader2, Film, Star, Check, Calendar, Clock,
  Eye, EyeOff, Trash2, ExternalLink, Pencil,
  Bookmark, BookmarkCheck, RefreshCw, ChevronRight,
  FolderOpen, HardDrive, Info, History, FileVideo,
  Download, Search, Users, Clapperboard,
  Archive, ArchiveRestore, HardDriveDownload,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { toast } from "sonner";
import { StatusBadge } from "./status-badge";
import { ReleaseSearchDialog } from "@/components/search/release-search-dialog";
import { autoSearch } from "@/lib/autosearch-api";
import { AltTitlesSection } from "@/components/alt-titles";
import type { Library } from "../../lib/libraries-api";
import type { Movie, QualityProfile, Credits, CreditPerson } from "./types";
import { TMDB_IMG } from "./types";

// ─── Collapsible Section ──────────────────────────────────────────────

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

// ─── Person Components ────────────────────────────────────────────────

const PROFILE_IMG = "https://image.tmdb.org/t/p/w185";

function PersonCard({ person }: { person: CreditPerson }) {
  return (
    <div className="flex-shrink-0 w-[100px] group">
      <div className="relative w-[100px] h-[150px] rounded-lg overflow-hidden bg-muted/30 mb-1.5">
        {person.profile_path ? (
          <img
            src={`${PROFILE_IMG}${person.profile_path}`}
            alt={person.name}
            className="w-full h-full object-cover"
            loading="lazy"
          />
        ) : (
          <div className="w-full h-full flex items-center justify-center text-muted-foreground/30">
            <Users className="w-8 h-8" />
          </div>
        )}
      </div>
      <p className="text-xs font-medium truncate" title={person.name}>{person.name}</p>
      {person.role && (
        <p className="text-[11px] text-muted-foreground truncate" title={person.role}>{person.role}</p>
      )}
    </div>
  );
}

function PersonChip({ person }: { person: CreditPerson }) {
  return (
    <div className="flex items-center gap-2 bg-muted/30 rounded-full pr-3 overflow-hidden">
      {person.profile_path ? (
        <img
          src={`${PROFILE_IMG}${person.profile_path}`}
          alt={person.name}
          className="w-8 h-8 rounded-full object-cover"
          loading="lazy"
        />
      ) : (
        <div className="w-8 h-8 rounded-full bg-muted/50 flex items-center justify-center">
          <Users className="w-4 h-4 text-muted-foreground/40" />
        </div>
      )}
      <span className="text-sm font-medium">{person.name}</span>
    </div>
  );
}

// ─── Movie Detail Sheet ───────────────────────────────────────────────

export function MovieDetailSheet({
  movie,
  open,
  onOpenChange,
  profiles,
  libraries,
  onUpdated,
  onDeleted,
}: {
  movie: Movie | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  profiles: QualityProfile[];
  libraries: Library[];
  onUpdated: (updated: Movie) => void;
  onDeleted: (id: string) => void;
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
  const [searchOpen, setSearchOpen] = useState(false);
  const [archiving, setArchiving] = useState(false);
  const [autoSearching, setAutoSearching] = useState(false);
  const [rescanning, setRescanning] = useState(false);

  const handleAutoSearch = async () => {
    if (!movie) return;
    setAutoSearching(true);
    const toastId = toast.loading(`Searching for ${movie.title}...`);
    try {
      const result = await autoSearch({
        media_type: "movie",
        media_id: movie.id,
        title: movie.title,
        quality_profile_id: movie.qualityProfileId,
        imdb_id: movie.imdbId || undefined,
        tmdb_id: movie.tmdbId || undefined,
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

  useEffect(() => {
    if (movie && open) {
      setEditing(false);
      setEditProfile(movie.qualityProfileId);
      setEditMonitoring(movie.monitoringStatus);
      setOverviewExpanded(false);
      // Fetch credits
      setCredits(null);
      setCreditsLoading(true);
      fetch(`/api/v1/movies/${movie.id}/credits`, { credentials: "include" })
        .then(r => r.ok ? r.json() : null)
        .then(data => setCredits(data))
        .catch((err) => console.error("fetch failed:", err))
        .finally(() => setCreditsLoading(false));
    }
  }, [movie, open]);

  if (!movie) return null;

  const profile = profiles.find(p => p.id === movie.qualityProfileId);
  const library = libraries.find(l => l.id === movie.libraryId);
  const isMonitored = movie.monitoringStatus === "monitored";
  const moviePath = library ? `${library.path}/${movie.title} (${movie.year})` : null;

  const handleSave = async () => {
    setSaving(true);
    try {
      const res = await fetch(`/api/v1/movies/${movie.id}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({
          quality_profile_id: editProfile,
          monitoring_status: editMonitoring,
        }),
      });
      if (res.ok) {
        const updated = await res.json();
        onUpdated(updated);
        setEditing(false);
        toast.success("Movie updated");
      } else {
        toast.error("Failed to update movie");
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
      const res = await fetch(`/api/v1/movies/${movie.id}/monitoring`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ status: newStatus }),
      });
      if (res.ok) {
        onUpdated({ ...movie, monitoringStatus: newStatus });
        toast.success(newStatus === "monitored" ? "Now monitoring" : "Unmonitored");
      }
    } catch {
      toast.error("Failed to update monitoring");
    }
  };

  const handleDelete = async () => {
    setDeleting(true);
    try {
      const res = await fetch(`/api/v1/movies/${movie.id}`, {
        method: "DELETE",
        credentials: "include",
      });
      if (res.ok || res.status === 204) {
        onDeleted(movie.id);
        onOpenChange(false);
        setDeleteOpen(false);
        toast.success("Movie deleted");
      } else {
        toast.error("Failed to delete movie");
      }
    } catch {
      toast.error("Network error");
    } finally {
      setDeleting(false);
    }
  };

  const handleArchiveToggle = async () => {
    if (!movie) return;
    setArchiving(true);
    const isArchived = movie.monitoringStatus === "unmonitored";
    const endpoint = isArchived
      ? `/api/v1/movies/${movie.id}/unarchive`
      : `/api/v1/movies/${movie.id}/archive`;
    try {
      const res = await fetch(endpoint, { method: "POST", credentials: "include" });
      if (res.ok) {
        const newStatus = isArchived ? "monitored" : "unmonitored";
        onUpdated({ ...movie, monitoringStatus: newStatus });
        toast.success(isArchived ? "Movie unarchived" : "Movie archived");
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
      const refreshRes = await fetch(`/api/v1/movies/${movie.id}/refresh`, { method: "POST", credentials: "include" });
      if (!refreshRes.ok) {
        toast.error("Failed to refresh");
        return;
      }
      const res = await fetch(`/api/v1/movies/${movie.id}`, { credentials: "include" });
      if (res.ok) {
        const updated = await res.json();
        onUpdated(updated);
        toast.success("Movie refreshed");
      } else {
        toast.error("Failed to fetch updated movie");
      }
    } catch {
      toast.error("Network error");
    } finally {
      setRefreshing(false);
    }
  };

  const handleRescan = async () => {
    if (!movie.libraryId) {
      toast.error("Movie has no library assigned");
      return;
    }
    setRescanning(true);
    try {
      const res = await fetch(`/api/v1/movies/${movie.id}/rescan`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ libraryId: movie.libraryId }),
      });
      if (res.ok) {
        const result = await res.json();
        toast.success(`Rescan complete: ${result.matched} matched, ${result.imported} imported`);
        const updated = await fetch(`/api/v1/movies/${movie.id}`, { credentials: "include" });
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

  const overviewIsLong = (movie.overview?.length ?? 0) > 280;

  return (
    <>
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent side="right" className="w-full sm:max-w-2xl p-0 overflow-y-auto">
          <SheetHeader className="sr-only">
            <SheetTitle>{movie.title}</SheetTitle>
            <SheetDescription>Movie details for {movie.title}</SheetDescription>
          </SheetHeader>

          {/* ── 1. Full-width backdrop header ── */}
          <div className="relative h-[300px] bg-muted overflow-hidden">
            {movie.backdropPath ? (
              <img src={`${TMDB_IMG}/w780${movie.backdropPath}`} alt="" className="w-full h-full object-cover" />
            ) : movie.posterPath ? (
              <img src={`${TMDB_IMG}/w780${movie.posterPath}`} alt="" className="w-full h-full object-cover opacity-30 blur-md scale-110" />
            ) : null}
            <div className="absolute inset-0 bg-gradient-to-t from-background via-background/70 to-black/30" />
            <div className="absolute inset-0 bg-gradient-to-r from-background/80 via-transparent to-transparent" />

            {/* Poster overlapping backdrop */}
            <div className="absolute bottom-[-40px] left-6 z-20">
              <div className="w-[130px] rounded-lg overflow-hidden shadow-2xl border-4 border-background">
                {movie.posterPath ? (
                  <img src={`${TMDB_IMG}/w300${movie.posterPath}`} alt={movie.title} className="w-full aspect-[2/3] object-cover" />
                ) : (
                  <div className="w-full aspect-[2/3] bg-muted flex items-center justify-center">
                    <Film className="w-10 h-10 text-muted-foreground/30" />
                  </div>
                )}
              </div>
            </div>

            {/* Title overlaid on backdrop */}
            <div className="absolute bottom-4 left-[170px] right-6 z-10">
              <h2 className="text-2xl font-bold text-white truncate drop-shadow-lg">{movie.title}</h2>
              <p className="text-sm text-white/70 mt-0.5 drop-shadow">{movie.year > 0 && movie.year}</p>
            </div>

            {/* Monitoring toggle — bookmark style */}
            <button
              onClick={handleToggleMonitoring}
              className={cn(
                "absolute top-4 right-4 z-20 p-2 rounded-full transition-all duration-200 shadow-lg",
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

          {/* ── Action Buttons toolbar ── */}
          <div className="px-6 pb-2">
            <div className="flex items-center gap-2 flex-wrap">
              <Button size="sm" variant="outline" className="gap-1.5" onClick={handleAutoSearch} disabled={autoSearching} title="Automated search (uses quality profile to pick the best result)">
                <Search className={cn("w-3.5 h-3.5", autoSearching && "animate-spin")} />{autoSearching ? "Searching..." : "Search"}
              </Button>
              <Button size="sm" variant="outline" className="gap-1.5" onClick={() => setSearchOpen(true)}>
                <Download className="w-3.5 h-3.5" />Interactive Search
              </Button>
              <Button size="sm" variant="outline" className="gap-1.5" onClick={() => { setEditing(true); setEditProfile(movie.qualityProfileId); setEditMonitoring(movie.monitoringStatus); }}>
                <Pencil className="w-3.5 h-3.5" />Edit
              </Button>
              <Button size="sm" variant="outline" className="gap-1.5" onClick={handleRefresh} disabled={refreshing}>
                <RefreshCw className={cn("w-3.5 h-3.5", refreshing && "animate-spin")} />Refresh
              </Button>
              <Button size="sm" variant="outline" className="gap-1.5" onClick={handleRescan} disabled={rescanning} title="Rescan library folder for new files">
                <HardDriveDownload className={cn("w-3.5 h-3.5", rescanning && "animate-spin")} />{rescanning ? "Scanning..." : "Rescan"}
              </Button>
              <Button size="sm" variant="outline" className="gap-1.5" onClick={handleArchiveToggle} disabled={archiving}>
                {movie.monitoringStatus === "unmonitored" ? (
                  <><ArchiveRestore className="w-3.5 h-3.5" />Unarchive</>
                ) : (
                  <><Archive className="w-3.5 h-3.5" />Archive</>
                )}
              </Button>
              <Button size="sm" variant="destructive" className="gap-1.5 ml-auto" onClick={() => setDeleteOpen(true)}>
                <Trash2 className="w-3.5 h-3.5" />Delete
              </Button>
            </div>
          </div>

          {/* ── Edit mode bar ── */}
          {editing && (
            <div className="mx-6 mb-3 p-4 rounded-lg border border-accent/30 bg-accent/5 space-y-4">
              <div className="flex items-center gap-2 text-sm font-semibold text-accent">
                <Pencil className="w-4 h-4" />Editing Movie
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-1.5">
                  <label className="text-xs font-medium text-muted-foreground">Quality Profile</label>
                  <Select value={editProfile} onValueChange={setEditProfile}>
                    <SelectTrigger className="h-9 text-sm"><SelectValue /></SelectTrigger>
                    <SelectContent>
                      {profiles.map(p => <SelectItem key={p.id} value={p.id}>{p.name}</SelectItem>)}
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-1.5">
                  <label className="text-xs font-medium text-muted-foreground">Monitoring</label>
                  <Select value={editMonitoring} onValueChange={setEditMonitoring}>
                    <SelectTrigger className="h-9 text-sm"><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="monitored">Monitored</SelectItem>
                      <SelectItem value="unmonitored">Unmonitored</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>
              <div className="flex gap-2 pt-1">
                <Button size="sm" onClick={handleSave} disabled={saving} className="gap-1.5">
                  {saving ? <Loader2 className="w-4 h-4 animate-spin" /> : <><Check className="w-3.5 h-3.5" />Save Changes</>}
                </Button>
                <Button size="sm" variant="ghost" onClick={() => setEditing(false)}>Cancel</Button>
              </div>
            </div>
          )}

          {/* ── Info Bar ── */}
          <div className="px-6 py-3 border-t border-b border-border/40 bg-muted/30">
            <div className="flex items-center gap-3 flex-wrap text-sm text-muted-foreground">
              {movie.runtime > 0 && (
                <span className="flex items-center gap-1">
                  <Clock className="w-3.5 h-3.5" />{Math.floor(movie.runtime / 60)}h {movie.runtime % 60}m
                </span>
              )}
              {movie.rating > 0 && (
                <span className="flex items-center gap-1">
                  <Star className="w-3.5 h-3.5 text-yellow-400 fill-yellow-400" />
                  <span className="text-yellow-400 font-medium">{movie.rating.toFixed(1)}</span>
                  <span className="text-muted-foreground/60">/10</span>
                </span>
              )}
              {movie.releaseDate && (
                <span className="flex items-center gap-1">
                  <Calendar className="w-3.5 h-3.5" />{movie.releaseDate}
                </span>
              )}
              <span className="mx-1 text-border">|</span>
              <StatusBadge status={movie.status} />
            </div>
            {movie.genres?.length > 0 && (
              <div className="flex gap-1.5 mt-2 flex-wrap">
                {movie.genres.map(g => (
                  <Badge key={g} variant="secondary" className="text-[10px] cursor-default">{g}</Badge>
                ))}
              </div>
            )}
          </div>

          {/* Scrollable sections */}
          <div className="px-6 pt-4 pb-8 space-y-1">
            {/* ── Overview ── */}
            <CollapsibleSection title="Overview" icon={Info} defaultOpen>
              <div className="relative">
                <p className={cn(
                  "text-sm text-muted-foreground leading-relaxed",
                  !overviewExpanded && overviewIsLong && "line-clamp-3",
                )}>
                  {movie.overview || "No overview available."}
                </p>
                {overviewIsLong && (
                  <button
                    onClick={() => setOverviewExpanded(v => !v)}
                    className="text-xs text-accent hover:underline mt-1.5 font-medium"
                  >
                    {overviewExpanded ? "Show Less" : "Show More"}
                  </button>
                )}
              </div>
            </CollapsibleSection>

            {/* ── Alt Titles ── */}
            <AltTitlesSection mediaId={movie.id} mediaType="movie" />

            {/* ── Details Panel ── */}
            <CollapsibleSection title="Details" icon={HardDrive} defaultOpen>
              <div className="grid grid-cols-2 gap-x-6 gap-y-4 text-sm">
                <DetailRow label="Quality Profile">
                  <div className="flex items-center gap-1.5">
                    <span className="inline-block w-2 h-2 rounded-full bg-accent shrink-0" />
                    {profile?.name ?? "—"}
                  </div>
                </DetailRow>
                <DetailRow label="Library">
                  <span className="flex items-center gap-1 truncate" title={library?.name}>
                    <FolderOpen className="w-3.5 h-3.5 text-muted-foreground shrink-0" />
                    {library?.name ?? "—"}
                  </span>
                </DetailRow>
                <DetailRow label="Status">
                  <StatusBadge status={movie.status} />
                </DetailRow>
                <DetailRow label="Monitoring">
                  <span className={cn("flex items-center gap-1.5", isMonitored ? "text-accent" : "text-muted-foreground")}>
                    {isMonitored ? <Eye className="w-3.5 h-3.5" /> : <EyeOff className="w-3.5 h-3.5" />}
                    {isMonitored ? "Monitored" : "Unmonitored"}
                  </span>
                </DetailRow>
                <DetailRow label="Minimum Availability">
                  <span className="flex items-center gap-1">
                    <Calendar className="w-3.5 h-3.5 text-muted-foreground" />
                    {movie.releaseDate || "—"}
                  </span>
                </DetailRow>
                <DetailRow label="Added">
                  {movie.createdAt ? new Date(movie.createdAt).toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" }) : "—"}
                </DetailRow>
                {moviePath && (
                  <div className="col-span-2">
                    <DetailRow label="Path">
                      <span className="flex items-center gap-1 text-xs font-mono truncate" title={moviePath}>
                        <FolderOpen className="w-3.5 h-3.5 text-muted-foreground shrink-0" />
                        {moviePath}
                      </span>
                    </DetailRow>
                  </div>
                )}
                {movie.tmdbId && (
                  <DetailRow label="TMDB ID">
                    <a
                      href={`https://www.themoviedb.org/movie/${movie.tmdbId}`}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="flex items-center gap-1 text-accent hover:underline"
                    >
                      {movie.tmdbId}<ExternalLink className="w-3 h-3" />
                    </a>
                  </DetailRow>
                )}
                {movie.imdbId && (
                  <DetailRow label="IMDB ID">
                    <a
                      href={`https://www.imdb.com/title/${movie.imdbId}`}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="flex items-center gap-1 text-accent hover:underline"
                    >
                      {movie.imdbId}<ExternalLink className="w-3 h-3" />
                    </a>
                  </DetailRow>
                )}
              </div>
            </CollapsibleSection>

            {/* ── External Links ── */}
            {(movie.tmdbId || movie.imdbId) && (
              <div className="border-t border-border/40 py-3">
                <div className="flex items-center gap-3 flex-wrap">
                  {movie.tmdbId && (
                    <a
                      href={`https://www.themoviedb.org/movie/${movie.tmdbId}`}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-medium bg-[#01b4e4]/10 text-[#01b4e4] hover:bg-[#01b4e4]/20 transition-colors"
                    >
                      <ExternalLink className="w-3 h-3" />TMDB
                    </a>
                  )}
                  {movie.imdbId && (
                    <a
                      href={`https://www.imdb.com/title/${movie.imdbId}`}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-bold bg-[#f5c518]/10 text-[#f5c518] hover:bg-[#f5c518]/20 transition-colors"
                    >
                      IMDb
                    </a>
                  )}
                </div>
              </div>
            )}

            {/* ── Movie Files ── */}
            <CollapsibleSection title="Movie Files" icon={FileVideo} defaultOpen={false}>
              <div className="flex flex-col items-center justify-center py-8 text-center">
                <FileVideo className="w-10 h-10 text-muted-foreground/20 mb-3" />
                <p className="text-sm text-muted-foreground">No movie files found</p>
                <p className="text-xs text-muted-foreground/60 mt-1">
                  Files will appear here once the movie is downloaded
                </p>
                <div className="flex gap-2 mt-4">
                  <Button size="sm" variant="outline" className="gap-1.5 text-xs" onClick={handleAutoSearch} disabled={autoSearching} title="Automated search">
                    <Search className={cn("w-3.5 h-3.5", autoSearching && "animate-spin")} />{autoSearching ? "Searching..." : "Search"}
                  </Button>
                  <Button size="sm" variant="outline" className="gap-1.5 text-xs" onClick={() => setSearchOpen(true)}>
                    <Download className="w-3.5 h-3.5" />Interactive Search
                  </Button>
                </div>
              </div>
            </CollapsibleSection>

            {/* ── History ── */}
            <CollapsibleSection title="History" icon={History} defaultOpen={false}>
              <div className="flex flex-col items-center justify-center py-8 text-center">
                <History className="w-10 h-10 text-muted-foreground/20 mb-3" />
                <p className="text-sm text-muted-foreground">No history available</p>
                <p className="text-xs text-muted-foreground/60 mt-1">
                  Download and import history will appear here
                </p>
              </div>
            </CollapsibleSection>

            {/* ── Cast & Crew ── */}
            <CollapsibleSection title="Cast & Crew" icon={Users} defaultOpen>
              {creditsLoading ? (
                <div className="flex items-center gap-2 py-4 text-sm text-muted-foreground">
                  <Loader2 className="w-4 h-4 animate-spin" /> Loading credits…
                </div>
              ) : credits && (credits.cast.length > 0 || credits.crew.length > 0) ? (
                <div className="space-y-4">
                  {/* Directors */}
                  {(() => {
                    const directors = credits.crew.filter(c => c.role === "Director");
                    if (directors.length === 0) return null;
                    return (
                      <div>
                        <h4 className="text-xs uppercase tracking-wider text-muted-foreground mb-2 flex items-center gap-1.5">
                          <Clapperboard className="w-3.5 h-3.5" /> Director{directors.length > 1 ? "s" : ""}
                        </h4>
                        <div className="flex gap-3 flex-wrap">
                          {directors.map(d => (
                            <PersonChip key={d.id} person={d} />
                          ))}
                        </div>
                      </div>
                    );
                  })()}

                  {/* Cast */}
                  {credits.cast.length > 0 && (
                    <div>
                      <h4 className="text-xs uppercase tracking-wider text-muted-foreground mb-2 flex items-center gap-1.5">
                        <Users className="w-3.5 h-3.5" /> Cast
                      </h4>
                      <div className="flex gap-3 overflow-x-auto pb-2 -mx-1 px-1 scrollbar-thin">
                        {credits.cast.slice(0, 20).map(c => (
                          <PersonCard key={c.id} person={c} />
                        ))}
                      </div>
                    </div>
                  )}

                  {/* Key Crew (Writers, Producers, Composers) */}
                  {(() => {
                    const keyCrew = credits.crew.filter(
                      c => c.role !== "Director" && ["Writing", "Production", "Sound"].includes(c.department)
                    );
                    if (keyCrew.length === 0) return null;
                    const grouped = keyCrew.reduce<Record<string, typeof keyCrew>>((acc, c) => {
                      (acc[c.department] ??= []).push(c);
                      return acc;
                    }, {});
                    return (
                      <div className="space-y-2">
                        {Object.entries(grouped).map(([dept, members]) => (
                          <div key={dept}>
                            <span className="text-xs text-muted-foreground">{dept}:</span>{" "}
                            <span className="text-sm">
                              {members.map((m, i) => (
                                <span key={m.id}>
                                  {m.name}
                                  {m.role !== dept ? <span className="text-muted-foreground/60"> ({m.role})</span> : null}
                                  {i < members.length - 1 ? ", " : ""}
                                </span>
                              ))}
                            </span>
                          </div>
                        ))}
                      </div>
                    );
                  })()}
                </div>
              ) : (
                <div className="flex flex-col items-center justify-center py-8 text-center">
                  <Users className="w-10 h-10 text-muted-foreground/20 mb-3" />
                  <p className="text-sm text-muted-foreground">No cast or crew information</p>
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
            <DialogTitle>Delete Movie</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete <strong>{movie.title}</strong>? This cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <div className="flex justify-end gap-2 mt-4">
            <Button variant="outline" onClick={() => setDeleteOpen(false)}>Cancel</Button>
            <Button variant="destructive" onClick={handleDelete} disabled={deleting}>
              {deleting ? <Loader2 className="w-4 h-4 animate-spin" /> : "Delete"}
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      {/* Release search dialog */}
      <ReleaseSearchDialog
        open={searchOpen}
        onOpenChange={setSearchOpen}
        title={movie.title}
        query={movie.title}
        tmdbId={movie.tmdbId ? Number(movie.tmdbId) : undefined}
        imdbId={movie.imdbId}
        mediaType="movie"
        movieId={movie.id}
        autoSearch
      />
    </>
  );
}
