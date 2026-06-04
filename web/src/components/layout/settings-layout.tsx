import * as React from "react";
import { Link, Outlet, useNavigate, useRouterState } from "@tanstack/react-router";
import {
  Bell,
  Bug,
  Download,
  Film,
  FolderOpen,
  Gauge,
  HeartPulse,
  Languages,
  ListPlus,
  Network,
  Palette,
  Radio,
  RefreshCw,
  Repeat,
  Rss,
  ScrollText,
  Share2,
  ShieldCheck,
  SlidersHorizontal,
  Tags,
  Terminal,
  ToggleRight,
  UsersRound,
  Workflow,
  type LucideIcon,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { useAuth } from "@/hooks/use-auth";
import { useFeatureEnabled } from "@/lib/features-api";
import { usePageHeader } from "@/hooks/use-page-header";

export interface SettingsNavItem {
  to: string;
  label: string;
  Icon: LucideIcon;
  /** "panel" items render a heading from this config; "page" items own their heading. */
  kind: "panel" | "page";
  description?: string;
  adminOnly?: boolean;
  feature?: string;
}

export interface SettingsNavGroup {
  id: string;
  label: string;
  items: SettingsNavItem[];
}

export const SETTINGS_GROUPS: SettingsNavGroup[] = [
  {
    id: "general",
    label: "General",
    items: [
      {
        to: "/settings/general",
        label: "General",
        Icon: SlidersHorizontal,
        kind: "panel",
        description: "Application name, logging, and core options.",
      },
      {
        to: "/settings/appearance",
        label: "Appearance",
        Icon: Palette,
        kind: "panel",
        description: "Theme, language, and interface preferences.",
      },
      {
        to: "/settings/features",
        label: "Features",
        Icon: ToggleRight,
        kind: "panel",
        description: "Enable or disable optional features.",
      },
    ],
  },
  {
    id: "media",
    label: "Media Management",
    items: [
      {
        to: "/settings/media-management",
        label: "Libraries & Naming",
        Icon: FolderOpen,
        kind: "panel",
        description: "Root folders, file naming, and import handling.",
      },
      {
        to: "/settings/media-preferences",
        label: "Media Preferences",
        Icon: Film,
        kind: "panel",
        description: "Defaults applied when adding new media.",
      },
      { to: "/settings/quality-profiles", label: "Quality Profiles", Icon: Gauge, kind: "page" },
      { to: "/settings/custom-formats", label: "Custom Formats", Icon: Tags, kind: "page" },
      { to: "/settings/language-profiles", label: "Language Profiles", Icon: Languages, kind: "page" },
    ],
  },
  {
    id: "downloads",
    label: "Downloads & Indexers",
    items: [
      {
        to: "/settings/download-clients",
        label: "Download Clients",
        Icon: Download,
        kind: "panel",
        description: "Connect torrent and usenet download clients.",
      },
      {
        to: "/settings/download-safety",
        label: "Download Safety",
        Icon: ShieldCheck,
        kind: "panel",
        description: "Guard against malicious or mislabeled releases.",
      },
      {
        to: "/settings/rolling-search",
        label: "Rolling Search",
        Icon: RefreshCw,
        kind: "panel",
        description: "Automatic background search for missing media.",
      },
      { to: "/settings/indexers", label: "Indexers", Icon: Radio, kind: "page" },
      { to: "/settings/sources", label: "RSS Feeds", Icon: Rss, kind: "page" },
      { to: "/settings/import-lists", label: "Import Lists", Icon: ListPlus, kind: "page" },
      { to: "/settings/proxies", label: "Proxies", Icon: Network, kind: "page" },
      { to: "/settings/search-queue", label: "Search Queue", Icon: Bug, kind: "page", feature: "search_log" },
    ],
  },
  {
    id: "integrations",
    label: "Integrations",
    items: [
      { to: "/settings/notifications", label: "Notifications", Icon: Bell, kind: "page" },
      {
        to: "/settings/connect",
        label: "Connect",
        Icon: Share2,
        kind: "panel",
        description: "Media servers (Plex, Jellyfin, Emby) and Trakt.",
      },
      {
        to: "/settings/sync-profiles",
        label: "Sync Profiles",
        Icon: Repeat,
        kind: "panel",
        description: "Map external lists and libraries to Loom.",
      },
    ],
  },
  {
    id: "system",
    label: "System",
    items: [
      { to: "/settings/health", label: "Indexer Health", Icon: HeartPulse, kind: "page" },
      { to: "/settings/events", label: "Events", Icon: ScrollText, kind: "page" },
      { to: "/settings/workflows", label: "Workflows", Icon: Workflow, kind: "page" },
      {
        to: "/settings/system",
        label: "System Logs",
        Icon: Terminal,
        kind: "panel",
        description: "Application logs and diagnostics.",
      },
      { to: "/settings/users", label: "Users", Icon: UsersRound, kind: "page", adminOnly: true },
    ],
  },
];

function isItemActive(path: string, to: string) {
  return path === to || path.startsWith(`${to}/`);
}

export function SettingsLayout() {
  const path = useRouterState({ select: (s) => s.location.pathname });
  const navigate = useNavigate();
  const { user } = useAuth();
  const isAdmin = user?.role === "admin";
  const searchLogEnabled = useFeatureEnabled("search_log");
  const { setHeader } = usePageHeader();

  const groups = React.useMemo(() => {
    const visible = (item: SettingsNavItem) =>
      (!item.adminOnly || isAdmin) &&
      (!item.feature || item.feature !== "search_log" || searchLogEnabled);
    return SETTINGS_GROUPS.map((g) => ({
      ...g,
      items: g.items.filter(visible),
    })).filter((g) => g.items.length > 0);
  }, [isAdmin, searchLogEnabled]);

  const flatItems = React.useMemo(() => groups.flatMap((g) => g.items), [groups]);
  const active = flatItems.find((i) => isItemActive(path, i.to));

  // Panel routes don't set their own header; do it here. Page routes set their own.
  React.useEffect(() => {
    if (active?.kind === "panel") {
      setHeader({ title: "Settings", subtitle: active.label });
    }
  }, [active, setHeader]);

  // The settings layout stays mounted while sub-routes swap, so the sticky
  // sub-nav (an independent overflow-y-auto scroll container) otherwise keeps
  // whatever scroll position the browser left it in — e.g. clicking a lower
  // item focus-scrolls the container, pushing the top groups out of view and
  // leaving them hidden after navigation. Deterministically reset the nav to
  // the top on each section change, then only scroll down far enough to reveal
  // the active item when it sits below the first screenful.
  const navRef = React.useRef<HTMLElement>(null);
  const activeTo = active?.to;
  React.useLayoutEffect(() => {
    window.scrollTo({ top: 0 });
    const nav = navRef.current;
    if (!nav) return;
    nav.scrollTop = 0;
    const activeEl = nav.querySelector<HTMLElement>('[aria-current="page"]');
    if (!activeEl) return;
    const top =
      activeEl.getBoundingClientRect().top -
      nav.getBoundingClientRect().top +
      nav.scrollTop;
    if (top + activeEl.offsetHeight > nav.clientHeight) {
      nav.scrollTop = top - 16;
    }
  }, [activeTo]);

  return (
    <div className="grid gap-6 lg:grid-cols-[15rem_1fr]">
      {/* Mobile section picker */}
      <div className="lg:hidden">
        <label htmlFor="settings-jump" className="sr-only">
          Settings section
        </label>
        <select
          id="settings-jump"
          value={active?.to ?? ""}
          onChange={(e) => navigate({ to: e.target.value })}
          className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
        >
          {groups.map((g) => (
            <optgroup key={g.id} label={g.label}>
              {g.items.map((i) => (
                <option key={i.to} value={i.to}>
                  {i.label}
                </option>
              ))}
            </optgroup>
          ))}
        </select>
      </div>

      {/* Desktop grouped sub-nav */}
      <nav
        ref={navRef}
        aria-label="Settings sections"
        className="hidden lg:block lg:sticky lg:top-20 lg:self-start lg:max-h-[calc(100vh-6rem)] lg:overflow-y-auto scrollbar-thin"
      >
        <div className="flex flex-col gap-4">
          {groups.map((g) => (
            <div key={g.id}>
              <p className="px-3 pb-1 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/60">
                {g.label}
              </p>
              <ul className="flex flex-col gap-0.5">
                {g.items.map(({ to, label, Icon }) => {
                  const isActive = isItemActive(path, to);
                  return (
                    <li key={to}>
                      <Link
                        to={to}
                        aria-current={isActive ? "page" : undefined}
                        className={cn(
                          "flex items-center gap-2.5 rounded-md px-3 py-1.5 text-[13px] font-medium transition-colors",
                          isActive
                            ? "bg-accent/15 text-accent border-l-2 border-accent"
                            : "text-muted-foreground hover:bg-accent/8 hover:text-foreground",
                        )}
                      >
                        <Icon className="h-4 w-4 shrink-0" aria-hidden="true" />
                        <span className="truncate">{label}</span>
                      </Link>
                    </li>
                  );
                })}
              </ul>
            </div>
          ))}
        </div>
      </nav>

      {/* Content */}
      <div className="min-w-0">
        <div key={active?.to ?? path} className="page-enter">
          {active?.kind === "panel" && (
            <header className="mb-5">
              <h1 className="text-xl font-semibold tracking-tight">{active.label}</h1>
              {active.description && (
                <p className="mt-1 text-sm text-muted-foreground">{active.description}</p>
              )}
            </header>
          )}
          <Outlet />
        </div>
      </div>
    </div>
  );
}
