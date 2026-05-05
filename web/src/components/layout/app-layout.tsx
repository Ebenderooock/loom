import * as React from "react";
import { Link, Outlet, useRouterState } from "@tanstack/react-router";
import {
  Bell,
  Calendar,
  Download,
  Film,
  Languages,
  LayoutDashboard,
  Library,
  ListPlus,
  ListTodo,
  Menu,
  Network,
  HeartPulse,
  Radio,
  Search,
  Settings,
  SlidersHorizontal,
  Rss,
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
import { PageHeaderProvider, usePageHeader } from "@/hooks/use-page-header";

interface NavItem {
  to:
    | "/"
    | "/library"
    | "/movies"
    | "/series"
    | "/activity"
    | "/calendar"
    | "/indexers"
    | "/indexers/health"
    | "/downloads"
    | "/sources"
    | "/proxies"
    | "/notifications"
    | "/language-profiles"
    | "/custom-formats"
    | "/import-lists"
    | "/settings";
  label: string;
  Icon: typeof LayoutDashboard;
}

const NAV: NavItem[] = [
  { to: "/", label: "Dashboard", Icon: LayoutDashboard },
  { to: "/library", label: "Library", Icon: Library },
  { to: "/movies", label: "Movies", Icon: Film },
  { to: "/series", label: "TV Shows", Icon: Tv },
  { to: "/activity", label: "Activity", Icon: ListTodo },
  { to: "/calendar", label: "Calendar", Icon: Calendar },
  { to: "/indexers", label: "Indexers", Icon: Radio },
  { to: "/indexers/health", label: "Indexer Health", Icon: HeartPulse },
  { to: "/downloads", label: "Downloads", Icon: Download },
  { to: "/sources", label: "Sources", Icon: Rss },
  { to: "/proxies", label: "Proxies", Icon: Network },
  { to: "/notifications", label: "Notifications", Icon: Bell },
  { to: "/language-profiles", label: "Languages", Icon: Languages },
  { to: "/custom-formats", label: "Custom Formats", Icon: SlidersHorizontal },
  { to: "/import-lists", label: "Import Lists", Icon: ListPlus },
  { to: "/settings", label: "Settings", Icon: Settings },
];

function SidebarNav({
  collapsed,
  onNavigate,
}: {
  collapsed?: boolean;
  onNavigate?: () => void;
}) {
  const router = useRouterState();
  const path = router.location.pathname;

  const [reviewCount, setReviewCount] = React.useState(0);
  React.useEffect(() => {
    fetch("/api/v1/reviews/count")
      .then((r) => r.json())
      .then((b) => setReviewCount(b.count ?? 0))
      .catch(() => {});
    const interval = setInterval(() => {
      fetch("/api/v1/reviews/count")
        .then((r) => r.json())
        .then((b) => setReviewCount(b.count ?? 0))
        .catch(() => {});
    }, 30_000);
    return () => clearInterval(interval);
  }, []);

  return (
    <nav aria-label="Primary" className="flex flex-col gap-1 p-2">
      {NAV.map(({ to, label, Icon }) => {
        const active =
          to === "/" ? path === "/" : path === to || path.startsWith(`${to}/`);
        return (
          <Link
            key={to}
            to={to}
            onClick={onNavigate}
            className={cn(
              "flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
              active
                ? "bg-accent text-accent-foreground"
                : "text-muted-foreground hover:bg-accent/50 hover:text-foreground",
              collapsed && "justify-center px-2",
            )}
            aria-current={active ? "page" : undefined}
          >
            <Icon className="h-4 w-4" aria-hidden="true" />
            {!collapsed && (
              <span className="flex items-center gap-2">
                {label}
                {to === "/activity" && reviewCount > 0 && (
                  <Badge
                    variant="destructive"
                    className="h-5 min-w-[1.25rem] px-1 text-xs"
                  >
                    {reviewCount}
                  </Badge>
                )}
              </span>
            )}
          </Link>
        );
      })}
    </nav>
  );
}

function Brand({ collapsed }: { collapsed?: boolean }) {
  return (
    <div className="flex h-14 items-center gap-2 border-b border-border px-4">
      <div
        aria-hidden="true"
        className="flex h-8 w-8 items-center justify-center rounded-md bg-primary text-primary-foreground"
      >
        <span className="text-sm font-bold">L</span>
      </div>
      {!collapsed && <span className="text-lg font-semibold">Loom</span>}
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
          "hidden shrink-0 border-r border-border bg-card md:flex md:flex-col",
          collapsed ? "md:w-16" : "md:w-60",
        )}
      >
        <Brand collapsed={collapsed} />
        <SidebarNav collapsed={collapsed} />
        <div className="mt-auto border-t border-border p-2">
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
        <header className="sticky top-0 z-30 flex h-14 items-center gap-2 border-b border-border bg-background/80 px-4 backdrop-blur">
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

              {/* Page title + subtitle */}
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
                  className="gap-2"
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

        <main id="main" className="min-w-0 flex-1 overflow-x-hidden p-4 md:p-6">
          {children ?? <Outlet />}
        </main>
      </div>

    </div>
  );
}
