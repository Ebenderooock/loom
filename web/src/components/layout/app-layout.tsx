import * as React from "react";
import { Link, Outlet, useRouterState } from "@tanstack/react-router";
import { apiFetch } from "@/lib/fetch";
import {
  Calendar,
  ChevronDown,
  Download,
  Film,
  FolderOpen,
  LayoutDashboard,
  ListPlus,
  ListTodo,
  Menu,
  HeartPulse,
  Radio,
  ScrollText,
  Search,
  Settings,
  Rss,
  Tv,
  Workflow,
  Bug,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { ThemeToggle } from "@/components/theme-toggle";
import {
  CommandPalette,
  useCommandPalette,
} from "@/components/command-palette";
import { cn } from "@/lib/utils";
import { PageHeaderProvider, usePageHeader } from "@/hooks/use-page-header";

interface NavItem {
  to: string;
  label: string;
  Icon: typeof LayoutDashboard;
  badge?: "review";
}

interface NavSection {
  id: string;
  label: string;
  items: NavItem[];
}

const NAV_SECTIONS: NavSection[] = [
  {
    id: "main",
    label: "",
    items: [
      { to: "/", label: "Dashboard", Icon: LayoutDashboard },
    ],
  },
  {
    id: "media",
    label: "Media",
    items: [
      { to: "/movies", label: "Movies", Icon: Film },
      { to: "/series", label: "TV Shows", Icon: Tv },
      { to: "/calendar", label: "Calendar", Icon: Calendar },
      { to: "/library", label: "Library", Icon: FolderOpen },
    ],
  },
  {
    id: "activity",
    label: "Activity",
    items: [
      { to: "/downloads", label: "Downloads", Icon: Download },
      { to: "/activity", label: "History", Icon: ListTodo, badge: "review" },
      { to: "/workflows", label: "Workflows", Icon: Workflow },
    ],
  },
  {
    id: "search",
    label: "Search",
    items: [
      { to: "/indexers", label: "Indexers", Icon: Radio },
      { to: "/sources", label: "RSS Feeds", Icon: Rss },
      { to: "/import-lists", label: "Import Lists", Icon: ListPlus },
      { to: "/search-debug", label: "Search Debug", Icon: Bug },
    ],
  },
  {
    id: "system",
    label: "System",
    items: [
      { to: "/indexers/health", label: "Health", Icon: HeartPulse },
      { to: "/events", label: "Events", Icon: ScrollText },
      { to: "/settings", label: "Settings", Icon: Settings },
    ],
  },
];

function useReviewCount() {
  const [count, setCount] = React.useState(0);
  React.useEffect(() => {
    const load = () =>
      apiFetch("/api/v1/reviews/count")
        .then((r) => r.json())
        .then((b) => setCount(b.count ?? 0))
        .catch((err) => console.error("fetch failed:", err));
    load();
    const interval = setInterval(load, 30_000);
    return () => clearInterval(interval);
  }, []);
  return count;
}

function SidebarNav({
  collapsed,
  onNavigate,
}: {
  collapsed?: boolean;
  onNavigate?: () => void;
}) {
  const router = useRouterState();
  const path = router.location.pathname;
  const reviewCount = useReviewCount();

  // Track which sections are expanded — all open by default
  const [openSections, setOpenSections] = React.useState<Record<string, boolean>>(() =>
    Object.fromEntries(NAV_SECTIONS.map((s) => [s.id, true]))
  );

  const toggleSection = (id: string) =>
    setOpenSections((prev) => ({ ...prev, [id]: !prev[id] }));

  // Auto-expand section containing the active route
  React.useEffect(() => {
    for (const section of NAV_SECTIONS) {
      const hasActive = section.items.some(({ to }) =>
        to === "/" ? path === "/" : path === to || path.startsWith(`${to}/`)
      );
      if (hasActive && !openSections[section.id]) {
        setOpenSections((prev) => ({ ...prev, [section.id]: true }));
      }
    }
  }, [path]);

  return (
    <nav aria-label="Primary" className="flex flex-col gap-0.5 p-2 overflow-y-auto flex-1 min-h-0 scrollbar-thin">
      {NAV_SECTIONS.map((section) => {
        const isOpen = openSections[section.id];
        const hasLabel = section.label !== "";

        return (
          <div key={section.id}>
            {/* Section header */}
            {hasLabel && !collapsed && (
              <button
                onClick={() => toggleSection(section.id)}
                className="flex w-full items-center justify-between px-3 py-1.5 mt-2 mb-0.5 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/60 hover:text-muted-foreground transition-colors"
              >
                {section.label}
                <ChevronDown
                  className={cn(
                    "h-3 w-3 transition-transform duration-200",
                    !isOpen && "-rotate-90"
                  )}
                />
              </button>
            )}

            {/* Collapsed: show a thin divider between sections */}
            {hasLabel && collapsed && (
              <div className="mx-2 my-2 border-t border-border/50" />
            )}

            {/* Nav items */}
            {(isOpen || collapsed || !hasLabel) &&
              section.items.map(({ to, label, Icon, badge }) => {
                const active =
                  to === "/" ? path === "/" : path === to || path.startsWith(`${to}/`);
                return (
                  <Link
                    key={to}
                    to={to}
                    onClick={onNavigate}
                    title={collapsed ? label : undefined}
                    className={cn(
                      "flex items-center gap-2.5 rounded-md px-3 py-1.5 text-[13px] font-medium transition-colors",
                      active
                        ? "bg-accent/15 text-accent border-l-2 border-accent shadow-sm shadow-accent/5"
                        : "text-muted-foreground hover:bg-accent/8 hover:text-foreground",
                      collapsed && "justify-center px-2",
                    )}
                    aria-current={active ? "page" : undefined}
                  >
                    <Icon className="h-4 w-4 shrink-0" aria-hidden="true" />
                    {!collapsed && (
                      <span className="flex items-center gap-2 truncate">
                        {label}
                        {badge === "review" && reviewCount > 0 && (
                          <Badge
                            variant="destructive"
                            className="h-4 min-w-[1rem] px-1 text-[10px] leading-none"
                          >
                            {reviewCount}
                          </Badge>
                        )}
                      </span>
                    )}
                  </Link>
                );
              })}
          </div>
        );
      })}
    </nav>
  );
}

function Brand({ collapsed }: { collapsed?: boolean }) {
  return (
    <div className="flex h-14 items-center gap-2.5 border-b border-border/50 px-4">
      <img src="/loom-logo.png" alt="" className="h-8 w-auto" aria-hidden="true" />
      {!collapsed && <span className="text-lg font-bold gradient-text">Loom</span>}
    </div>
  );
}

export function AppLayout({ children }: { children?: React.ReactNode }) {
  return (
    <PageHeaderProvider>
      <AppLayoutInner>{children}</AppLayoutInner>
    </PageHeaderProvider>
  );
}

function AppLayoutInner({ children }: { children?: React.ReactNode }) {
  const [collapsed, setCollapsed] = React.useState(false);
  const [mobileOpen, setMobileOpen] = React.useState(false);
  const { open: paletteOpen, setOpen: setPaletteOpen } = useCommandPalette();
  const { header } = usePageHeader();

  return (
    <div className="flex min-h-screen w-full bg-background text-foreground">
      <aside
        aria-label="Sidebar"
        className={cn(
          "hidden shrink-0 border-r border-border/50 bg-card/80 backdrop-blur-xl md:flex md:flex-col md:sticky md:top-0 md:h-screen md:min-h-0",
          collapsed ? "md:w-16" : "md:w-56",
        )}
      >
        <Brand collapsed={collapsed} />
        <SidebarNav collapsed={collapsed} />
        <div className="shrink-0 border-t border-border p-2">
          <Button
            variant="ghost"
            size="sm"
            className="w-full justify-start"
            onClick={() => setCollapsed((c) => !c)}
            aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
          >
            <Menu className="h-4 w-4" />
            {!collapsed && <span>Collapse</span>}
          </Button>
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="sticky top-0 z-30 flex h-14 items-center gap-2 border-b border-border/50 bg-background/70 px-4 backdrop-blur-xl">
          {!paletteOpen ? (
            <>
              <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
                <SheetTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="md:hidden"
                    aria-label="Open navigation"
                  >
                    <Menu className="h-5 w-5" />
                  </Button>
                </SheetTrigger>
                <SheetContent side="left" className="w-64 p-0">
                  <SheetHeader className="border-b border-border p-4">
                    <SheetTitle>Loom</SheetTitle>
                  </SheetHeader>
                  <SidebarNav onNavigate={() => setMobileOpen(false)} />
                </SheetContent>
              </Sheet>

              {/* Logo + page title */}
              <Link to="/" className="flex items-center gap-2 md:hidden" aria-label="Loom home">
                <img src="/loom-logo.png" alt="" className="h-7 w-auto" aria-hidden="true" />
                <span className="text-lg font-bold gradient-text">Loom</span>
              </Link>

              {header.title && (
                <div className="hidden md:flex items-baseline gap-2 min-w-0">
                  <span className="text-sm font-semibold whitespace-nowrap">{header.title}</span>
                  {header.subtitle && (
                    <span className="text-xs text-muted-foreground whitespace-nowrap truncate">{header.subtitle}</span>
                  )}
                </div>
              )}

              <div className="flex-1" />

              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  className="gap-2 border-border/50 hover:border-accent/30 hover:shadow-sm hover:shadow-accent/10"
                  onClick={() => setPaletteOpen(true)}
                  aria-label="Open command palette"
                >
                  <Search className="h-4 w-4" />
                  <span className="hidden sm:inline">Quick Search</span>
                  <kbd className="hidden rounded border border-border bg-muted px-1.5 py-0.5 font-mono text-[10px] font-medium sm:inline-block">
                    ⌘K
                  </kbd>
                </Button>
                <ThemeToggle />
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label="User menu"
                      className="rounded-full"
                    >
                      <span
                        aria-hidden="true"
                        className="flex h-8 w-8 items-center justify-center rounded-full bg-muted text-xs font-semibold"
                      >
                        LM
                      </span>
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuLabel>Signed in</DropdownMenuLabel>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem disabled>Profile</DropdownMenuItem>
                    <DropdownMenuItem disabled>Sign out</DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            </>
          ) : (
            <CommandPalette open={paletteOpen} onOpenChange={setPaletteOpen} inline />
          )}
        </header>

        {/* Dimmed backdrop when search is open */}
        {paletteOpen && (
          <div
            className="fixed inset-0 z-20 bg-black/50 backdrop-blur-[2px]"
            style={{ top: "3.5rem" }}
            onClick={() => setPaletteOpen(false)}
          />
        )}

        <main id="main" className="min-w-0 flex-1 overflow-x-hidden p-4 md:p-6 page-enter">
          {children ?? <Outlet />}
        </main>
      </div>

    </div>
  );
}
