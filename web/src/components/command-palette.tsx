import * as React from "react";
import { Command } from "cmdk";
import { useNavigate } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "@/lib/fetch";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Calendar,
  LayoutDashboard,
  Library,
  ListTodo,
  Search,
  Settings,
  X,
  Film,
  Tv,
  Compass,
  Download,
  Inbox,
  Radio,
  Database,
  ListChecks,
  Bell,
  SlidersHorizontal,
  Workflow,
  Loader2,
} from "lucide-react";

// ─── Navigation targets (everything reachable in the app) ────────────
interface NavItem {
  label: string;
  to: string;
  keywords?: string;
  Icon: typeof LayoutDashboard;
}

const navItems: NavItem[] = [
  {
    label: "Dashboard",
    to: "/",
    keywords: "home overview",
    Icon: LayoutDashboard,
  },
  { label: "Movies", to: "/movies", Icon: Film },
  { label: "TV Shows", to: "/series", keywords: "series shows tv", Icon: Tv },
  {
    label: "Discover",
    to: "/discover",
    keywords: "browse find new",
    Icon: Compass,
  },
  {
    label: "Requests",
    to: "/requests",
    keywords: "request media ask",
    Icon: Inbox,
  },
  { label: "Library", to: "/library", Icon: Library },
  {
    label: "Calendar",
    to: "/calendar",
    keywords: "schedule upcoming",
    Icon: Calendar,
  },
  {
    label: "Activity",
    to: "/activity",
    keywords: "history queue",
    Icon: ListTodo,
  },
  {
    label: "Downloads",
    to: "/downloads",
    keywords: "torrents clients",
    Icon: Download,
  },
  {
    label: "Settings",
    to: "/settings",
    keywords: "config preferences",
    Icon: Settings,
  },
  {
    label: "Indexers",
    to: "/settings/indexers",
    keywords: "settings trackers prowlarr",
    Icon: Radio,
  },
  {
    label: "RSS Feeds",
    to: "/settings/sources",
    keywords: "settings sources rss",
    Icon: Database,
  },
  {
    label: "Import Lists",
    to: "/settings/import-lists",
    keywords: "settings trakt lists rss",
    Icon: ListChecks,
  },
  {
    label: "Quality Profiles",
    to: "/settings/quality-profiles",
    keywords: "settings quality",
    Icon: SlidersHorizontal,
  },
  {
    label: "Custom Formats",
    to: "/settings/custom-formats",
    keywords: "settings scoring formats",
    Icon: SlidersHorizontal,
  },
  {
    label: "Notifications",
    to: "/settings/notifications",
    keywords: "settings discord webhook alerts",
    Icon: Bell,
  },
  {
    label: "Download Clients",
    to: "/settings/download-clients",
    keywords: "settings qbittorrent sabnzbd",
    Icon: Download,
  },
  {
    label: "Workflows",
    to: "/settings/workflows",
    keywords: "settings jobs tasks",
    Icon: Workflow,
  },
  {
    label: "Users",
    to: "/settings/users",
    keywords: "settings accounts admin",
    Icon: Settings,
  },
];

// ─── Content fetched for live search ─────────────────────────────────
interface ContentItem {
  id: string;
  title: string;
  year?: number;
}

async function fetchList(url: string): Promise<ContentItem[]> {
  const res = await apiFetch(url);
  if (!res.ok) return [];
  const data = await res.json();
  const arr: unknown[] = Array.isArray(data) ? data : (data?.data ?? []);
  return (arr as Array<Record<string, unknown>>).map((m) => ({
    id: String(m.id ?? ""),
    title: String(m.title ?? ""),
    year: typeof m.year === "number" ? m.year : undefined,
  }));
}

const RESULT_LIMIT = 8;

interface CommandPaletteProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  inline?: boolean;
}

function PaletteBody({
  query,
  setQuery,
  inline,
  inputRef,
  onClose,
}: {
  query: string;
  setQuery: (v: string) => void;
  inline?: boolean;
  inputRef?: React.RefObject<HTMLInputElement>;
  onClose: () => void;
}) {
  const navigate = useNavigate();
  const trimmed = query.trim();
  const hasQuery = trimmed.length > 0;
  const lower = trimmed.toLowerCase();

  // Only fetch content once the user starts searching.
  const { data: movies = [], isFetching: moviesLoading } = useQuery({
    queryKey: ["palette", "movies"],
    queryFn: () => fetchList("/api/v1/movies?limit=1000"),
    enabled: hasQuery,
    staleTime: 60_000,
  });
  const { data: series = [], isFetching: seriesLoading } = useQuery({
    queryKey: ["palette", "series"],
    queryFn: () => fetchList("/api/v1/series"),
    enabled: hasQuery,
    staleTime: 60_000,
  });

  const go = React.useCallback(
    (to: string, search?: Record<string, unknown>) => {
      onClose();
      void navigate({ to, search } as never);
    },
    [navigate, onClose],
  );

  const matchedNav = React.useMemo(() => {
    if (!hasQuery) return navItems;
    return navItems.filter(
      (n) =>
        n.label.toLowerCase().includes(lower) ||
        (n.keywords ?? "").includes(lower),
    );
  }, [hasQuery, lower]);

  const matchedMovies = React.useMemo(() => {
    if (!hasQuery) return [];
    return movies
      .filter(
        (m) =>
          m.title.toLowerCase().includes(lower) ||
          String(m.year ?? "").includes(lower),
      )
      .slice(0, RESULT_LIMIT);
  }, [hasQuery, lower, movies]);

  const matchedSeries = React.useMemo(() => {
    if (!hasQuery) return [];
    return series
      .filter(
        (s) =>
          s.title.toLowerCase().includes(lower) ||
          String(s.year ?? "").includes(lower),
      )
      .slice(0, RESULT_LIMIT);
  }, [hasQuery, lower, series]);

  const contentLoading = hasQuery && (moviesLoading || seriesLoading);

  const listClass = inline
    ? "absolute top-full left-0 right-0 max-h-96 overflow-y-auto bg-background border border-border border-t-0 rounded-b-lg shadow-xl p-2"
    : "max-h-96 overflow-y-auto p-2";

  return (
    <>
      {inline ? (
        <div className="flex flex-1 items-center gap-2">
          <Search className="h-4 w-4 shrink-0 text-muted-foreground" />
          <Command.Input
            ref={inputRef}
            value={query}
            onValueChange={setQuery}
            placeholder="Search Loom — movies, series, pages…"
            className="h-10 flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
          />
          <kbd className="rounded border border-border bg-muted px-1.5 py-0.5 font-mono text-[10px] font-medium text-muted-foreground">
            ESC
          </kbd>
          <button
            onClick={onClose}
            className="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
      ) : (
        <Command.Input
          value={query}
          onValueChange={setQuery}
          placeholder="Search Loom — movies, series, pages…"
          className="h-12 border-b border-border bg-transparent px-4 text-sm outline-none placeholder:text-muted-foreground"
        />
      )}

      <Command.List className={listClass}>
        <Command.Empty className="py-6 text-center text-sm text-muted-foreground">
          {contentLoading ? (
            <span className="inline-flex items-center gap-2">
              <Loader2 className="h-4 w-4 animate-spin" /> Searching…
            </span>
          ) : (
            "No results found."
          )}
        </Command.Empty>

        {contentLoading &&
          (matchedMovies.length > 0 ||
            matchedSeries.length > 0 ||
            matchedNav.length > 0) && (
            <div className="flex items-center gap-2 px-3 py-2 text-xs text-muted-foreground">
              <Loader2 className="h-3.5 w-3.5 animate-spin" /> Searching your
              library…
            </div>
          )}

        {matchedMovies.length > 0 && (
          <Command.Group
            heading="Movies"
            className="px-2 py-1 text-xs text-muted-foreground"
          >
            {matchedMovies.map((m) => (
              <Command.Item
                key={`movie-${m.id}`}
                value={`movie ${m.title} ${m.year ?? ""} ${m.id}`}
                onSelect={() => go("/movies", { focus: m.id })}
                className="flex cursor-pointer items-center gap-2 rounded-md px-3 py-2 text-sm aria-selected:bg-accent aria-selected:text-accent-foreground"
              >
                <Film className="h-4 w-4 shrink-0" />
                <span className="truncate">{m.title}</span>
                {m.year ? (
                  <span className="text-muted-foreground">({m.year})</span>
                ) : null}
              </Command.Item>
            ))}
          </Command.Group>
        )}

        {matchedSeries.length > 0 && (
          <Command.Group
            heading="TV Shows"
            className="px-2 py-1 text-xs text-muted-foreground"
          >
            {matchedSeries.map((s) => (
              <Command.Item
                key={`series-${s.id}`}
                value={`series ${s.title} ${s.year ?? ""} ${s.id}`}
                onSelect={() => go("/series", { focus: s.id })}
                className="flex cursor-pointer items-center gap-2 rounded-md px-3 py-2 text-sm aria-selected:bg-accent aria-selected:text-accent-foreground"
              >
                <Tv className="h-4 w-4 shrink-0" />
                <span className="truncate">{s.title}</span>
                {s.year ? (
                  <span className="text-muted-foreground">({s.year})</span>
                ) : null}
              </Command.Item>
            ))}
          </Command.Group>
        )}

        {matchedNav.length > 0 && (
          <Command.Group
            heading="Navigation"
            className="px-2 py-1 text-xs text-muted-foreground"
          >
            {matchedNav.map(({ label, to, Icon }) => (
              <Command.Item
                key={to}
                value={`nav ${label}`}
                onSelect={() => go(to)}
                className="flex cursor-pointer items-center gap-2 rounded-md px-3 py-2 text-sm aria-selected:bg-accent aria-selected:text-accent-foreground"
              >
                <Icon className="h-4 w-4 shrink-0" />
                <span>{label}</span>
              </Command.Item>
            ))}
          </Command.Group>
        )}
      </Command.List>
    </>
  );
}

export function CommandPalette({
  open,
  onOpenChange,
  inline,
}: CommandPaletteProps) {
  const [query, setQuery] = React.useState("");
  const inputRef = React.useRef<HTMLInputElement>(null);

  const close = React.useCallback(() => onOpenChange(false), [onOpenChange]);

  // Reset the query whenever the palette closes so it opens fresh next time.
  React.useEffect(() => {
    if (!open) setQuery("");
  }, [open]);

  React.useEffect(() => {
    if (open && inline) {
      requestAnimationFrame(() => inputRef.current?.focus());
    }
  }, [open, inline]);

  if (inline) {
    if (!open) return null;
    return (
      <Command
        label="Command Menu"
        shouldFilter={false}
        className="relative flex flex-1 flex-col"
      >
        <PaletteBody
          query={query}
          setQuery={setQuery}
          inline
          inputRef={inputRef}
          onClose={close}
        />
      </Command>
    );
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="overflow-hidden p-0">
        <DialogTitle className="sr-only">Command Palette</DialogTitle>
        <DialogDescription className="sr-only">
          Search Loom for movies, series, and pages.
        </DialogDescription>
        <Command
          label="Command Menu"
          shouldFilter={false}
          className="flex flex-col"
        >
          <PaletteBody query={query} setQuery={setQuery} onClose={close} />
        </Command>
      </DialogContent>
    </Dialog>
  );
}

export function useCommandPalette() {
  const [open, setOpen] = React.useState(false);

  React.useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.key === "k" || e.key === "K") && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        setOpen((v) => !v);
      }
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, []);

  return { open, setOpen };
}
