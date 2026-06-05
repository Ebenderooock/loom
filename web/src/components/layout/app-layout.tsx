import * as React from "react";
import { Link, Outlet, useRouterState } from "@tanstack/react-router";
import { apiFetch } from "@/lib/fetch";
import {
  Activity,
  Calendar,
  Compass,
  Inbox,
  Download,
  Film,
  FolderOpen,
  LayoutDashboard,
  ListTodo,
  Menu,
  Search,
  Settings,
  Tv,
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
import { useFeatureEnabled } from "@/lib/features-api";
import { useAuth } from "@/hooks/use-auth";
import { PageHeaderProvider, usePageHeader } from "@/hooks/use-page-header";

interface NavItem {
  to: string;
  label: string;
  Icon: typeof LayoutDashboard;
  badge?: "review";
}

const PRIMARY_NAV: NavItem[] = [
  { to: "/", label: "Dashboard", Icon: LayoutDashboard },
  { to: "/discover", label: "Discover", Icon: Compass },
  { to: "/requests", label: "Requests", Icon: Inbox },
  { to: "/movies", label: "Movies", Icon: Film },
  { to: "/series", label: "TV Shows", Icon: Tv },
  { to: "/calendar", label: "Calendar", Icon: Calendar },
  { to: "/library", label: "Library", Icon: FolderOpen },
  { to: "/activity", label: "Activity", Icon: ListTodo, badge: "review" },
  { to: "/downloads", label: "Downloads", Icon: Download },
];

const SETTINGS_NAV: NavItem = { to: "/settings", label: "Settings", Icon: Settings };

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

function NavLinkRow({
  item,
  active,
  collapsed,
  onNavigate,
  reviewCount,
}: {
  item: NavItem;
  active: boolean;
  collapsed?: boolean;
  onNavigate?: () => void;
  reviewCount: number;
}) {
  const { to, label, Icon, badge } = item;
  return (
    <Link
      to={to}
      onClick={onNavigate}
      title={collapsed ? label : undefined}
      className={cn(
        "flex items-center gap-2.5 rounded-md px-3 py-2 text-[13px] font-medium transition-colors",
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
  const { user } = useAuth();
  const analyticsEnabled = useFeatureEnabled("media_analytics");
  const showAnalytics = analyticsEnabled && user?.role === "admin";

  const navItems = React.useMemo(() => {
    if (!showAnalytics) return PRIMARY_NAV;
    return [
      ...PRIMARY_NAV,
      { to: "/analytics", label: "Analytics", Icon: Activity } as NavItem,
    ];
  }, [showAnalytics]);

  const isActive = (to: string) =>
    to === "/" ? path === "/" : path === to || path.startsWith(`${to}/`);

  return (
    <nav
      aria-label="Primary"
      className="flex flex-col gap-0.5 p-2 overflow-y-auto flex-1 min-h-0 scrollbar-thin"
    >
      {navItems.map((item) => (
        <NavLinkRow
          key={item.to}
          item={item}
          active={isActive(item.to)}
          collapsed={collapsed}
          onNavigate={onNavigate}
          reviewCount={reviewCount}
        />
      ))}

      <div className="mx-2 my-2 border-t border-border/50" />

      <NavLinkRow
        item={SETTINGS_NAV}
        active={isActive(SETTINGS_NAV.to)}
        collapsed={collapsed}
        onNavigate={onNavigate}
        reviewCount={reviewCount}
      />
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
  // Animate (and remount) only when the top-level section changes, so that
  // navigating between sub-pages within a section (e.g. Settings) swaps the
  // content without remounting the section's own layout/sub-nav.
  const sectionKey = useRouterState({
    select: (s) => "/" + (s.location.pathname.split("/")[1] ?? ""),
  });

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
            role="button"
            tabIndex={0}
            aria-label="Close search"
            onClick={() => setPaletteOpen(false)}
            onKeyDown={(e) => {
              if (e.key === "Enter" || e.key === " " || e.key === "Escape") {
                e.preventDefault();
                setPaletteOpen(false);
              }
            }}
          />
        )}

        <main id="main" className="min-w-0 flex-1 overflow-x-hidden p-4 md:p-6">
          <div key={sectionKey} className="page-enter">
            {children ?? <Outlet />}
          </div>
        </main>
      </div>

    </div>
  );
}
