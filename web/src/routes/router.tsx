import { Suspense, lazy } from "react";
import {
  createRootRoute,
  createRoute,
  createRouter,
  lazyRouteComponent,
  redirect,
  Outlet,
} from "@tanstack/react-router";
import { AppLayout } from "@/components/layout/app-layout";
import { SettingsLayout } from "@/components/layout/settings-layout";
import { NotFoundPage } from "@/pages/not-found";
import { ErrorFallback } from "@/components/ui/error-fallback";
import { useAuth } from "@/hooks/use-auth";
import { PageLoader } from "@/components/ui/page-loader";

// Lazy-loaded page components (route-level code splitting)
const SetupPage = lazy(() =>
  import("@/pages/setup").then((m) => ({ default: m.SetupPage })),
);
const AuthPage = lazy(() =>
  import("@/pages/auth").then((m) => ({ default: m.AuthPage })),
);

function RootComponent() {
  const { isSetupComplete, isAuthenticated, isLoading } = useAuth();

  if (isLoading) {
    return <div className="h-screen w-screen bg-neutral-dark" />;
  }

  if (!isSetupComplete) {
    return (
      <Suspense fallback={<PageLoader />}>
        <SetupPage />
      </Suspense>
    );
  }

  if (!isAuthenticated) {
    // Public self-service invite acceptance lives outside the auth gate.
    if (
      typeof window !== "undefined" &&
      window.location.pathname.startsWith("/invite/")
    ) {
      return <Outlet />;
    }
    return (
      <Suspense fallback={<PageLoader />}>
        <AuthPage />
      </Suspense>
    );
  }

  return <AppLayout />;
}

const rootRoute = createRootRoute({
  component: RootComponent,
  errorComponent: ErrorFallback,
  notFoundComponent: NotFoundPage,
});

const setupRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/setup",
  component: lazyRouteComponent(() => import("@/pages/setup"), "SetupPage"),
  pendingComponent: PageLoader,
  errorComponent: ErrorFallback,
});

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: lazyRouteComponent(
    () => import("@/pages/dashboard"),
    "DashboardPage",
  ),
  pendingComponent: PageLoader,
  errorComponent: ErrorFallback,
});

const libraryRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/library",
  component: lazyRouteComponent(() => import("@/pages/library"), "LibraryPage"),
  pendingComponent: PageLoader,
  errorComponent: ErrorFallback,
});

const moviesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/movies",
  component: lazyRouteComponent(() => import("@/pages/movies"), "MoviesPage"),
  pendingComponent: PageLoader,
  errorComponent: ErrorFallback,
  validateSearch: (search: Record<string, unknown>): { focus?: string } => ({
    focus: (search.focus as string) ?? undefined,
  }),
});

const seriesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/series",
  component: lazyRouteComponent(() => import("@/pages/series"), "SeriesPage"),
  pendingComponent: PageLoader,
  errorComponent: ErrorFallback,
  validateSearch: (search: Record<string, unknown>): { focus?: string } => ({
    focus: (search.focus as string) ?? undefined,
  }),
});

const musicRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/music",
  component: lazyRouteComponent(() => import("@/pages/music"), "MusicPage"),
  pendingComponent: PageLoader,
  errorComponent: ErrorFallback,
});

const musicArtistRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/music/$artistId",
  component: lazyRouteComponent(
    () => import("@/pages/music-artist"),
    "MusicArtistPage",
  ),
  pendingComponent: PageLoader,
  errorComponent: ErrorFallback,
});

const discoverRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/discover",
  component: lazyRouteComponent(
    () => import("@/pages/discover"),
    "DiscoverPage",
  ),
  pendingComponent: PageLoader,
  errorComponent: ErrorFallback,
});

const requestsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/requests",
  component: lazyRouteComponent(
    () => import("@/pages/requests"),
    "RequestsPage",
  ),
  pendingComponent: PageLoader,
  errorComponent: ErrorFallback,
});

const analyticsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/analytics",
  component: lazyRouteComponent(
    () => import("@/pages/analytics"),
    "AnalyticsPage",
  ),
  pendingComponent: PageLoader,
  errorComponent: ErrorFallback,
});

const activityRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/activity",
  component: lazyRouteComponent(
    () => import("@/pages/activity"),
    "ActivityPage",
  ),
  pendingComponent: PageLoader,
  errorComponent: ErrorFallback,
});

const calendarRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/calendar",
  component: lazyRouteComponent(
    () => import("@/pages/calendar"),
    "CalendarPage",
  ),
  pendingComponent: PageLoader,
  errorComponent: ErrorFallback,
});

const downloadsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/downloads",
  component: lazyRouteComponent(
    () => import("@/pages/downloads"),
    "DownloadsPage",
  ),
  pendingComponent: PageLoader,
  errorComponent: ErrorFallback,
});

// ─── Settings (layout + nested sections) ─────────────────────────────────

const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/settings",
  component: SettingsLayout,
  errorComponent: ErrorFallback,
});

const settingsIndexRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: "/",
  beforeLoad: () => {
    throw redirect({ to: "/settings/general", replace: true });
  },
});

function settingsChild<const P extends string>(
  path: P,
  loader: () => Promise<Record<string, unknown>>,
  exportName: string,
) {
  return createRoute({
    getParentRoute: () => settingsRoute,
    path,
    component: lazyRouteComponent(loader as never, exportName),
    pendingComponent: PageLoader,
    errorComponent: ErrorFallback,
  });
}

const settingsGeneralRoute = settingsChild(
  "general",
  () => import("@/pages/settings"),
  "GeneralPanel",
);
const settingsAppearanceRoute = settingsChild(
  "appearance",
  () => import("@/pages/settings"),
  "UIPanel",
);
const settingsFeaturesRoute = settingsChild(
  "features",
  () => import("@/pages/settings"),
  "FeaturesPanel",
);
const settingsMediaManagementRoute = settingsChild(
  "media-management",
  () => import("@/pages/settings"),
  "MediaManagementPanel",
);
const settingsMediaPreferencesRoute = settingsChild(
  "media-preferences",
  () => import("@/pages/settings"),
  "MediaPreferencesPanel",
);
const settingsQualityProfilesRoute = settingsChild(
  "quality-profiles",
  () => import("@/pages/quality-profiles"),
  "QualityProfilesPage",
);
const settingsCustomFormatsRoute = settingsChild(
  "custom-formats",
  () => import("@/pages/custom-formats"),
  "CustomFormatsPage",
);
const settingsMusicRoute = settingsChild(
  "music",
  () => import("@/pages/music-settings"),
  "MusicProfilesPage",
);
const settingsLanguageProfilesRoute = settingsChild(
  "language-profiles",
  () => import("@/pages/language-profiles"),
  "LanguageProfilesPage",
);
const settingsDownloadClientsRoute = settingsChild(
  "download-clients",
  () => import("@/pages/settings"),
  "DownloadClientsPanel",
);
const settingsDownloadSafetyRoute = settingsChild(
  "download-safety",
  () => import("@/pages/settings"),
  "DownloadSafetyPanel",
);
const settingsRollingSearchRoute = settingsChild(
  "rolling-search",
  () => import("@/pages/settings"),
  "RollingSearchPanel",
);
const settingsIndexersRoute = settingsChild(
  "indexers",
  () => import("@/pages/indexers"),
  "IndexersPage",
);
const settingsSourcesRoute = settingsChild(
  "sources",
  () => import("@/pages/sources"),
  "SourcesPage",
);
const settingsImportListsRoute = settingsChild(
  "import-lists",
  () => import("@/pages/import-lists"),
  "ImportListsPage",
);
const settingsProxiesRoute = settingsChild(
  "proxies",
  () => import("@/pages/proxies"),
  "ProxiesPage",
);
const settingsSearchQueueRoute = settingsChild(
  "search-queue",
  () => import("@/pages/search-debug"),
  "SearchDebugPage",
);
const settingsNotificationsRoute = settingsChild(
  "notifications",
  () => import("@/pages/notifications"),
  "NotificationsPage",
);
const settingsRequestBotsRoute = settingsChild(
  "request-bots",
  () => import("@/pages/request-bots"),
  "RequestBotsPage",
);
const settingsPluginsRoute = settingsChild(
  "plugins",
  () => import("@/pages/plugins"),
  "PluginsPage",
);
const settingsConnectRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: "connect",
  component: lazyRouteComponent(
    () => import("@/pages/settings"),
    "ConnectPanel",
  ),
  pendingComponent: PageLoader,
  errorComponent: ErrorFallback,
  validateSearch: (
    search: Record<string, unknown>,
  ): { trakt_code?: string } => ({
    trakt_code: (search.trakt_code as string) ?? undefined,
  }),
});
const settingsSyncProfilesRoute = settingsChild(
  "sync-profiles",
  () => import("@/components/settings/sync-profiles-panel"),
  "SyncProfilesPanel",
);
const settingsHealthRoute = settingsChild(
  "health",
  () => import("@/pages/indexer-health"),
  "IndexerHealthPage",
);
const settingsEventsRoute = settingsChild(
  "events",
  () => import("@/pages/events"),
  "EventsPage",
);
const settingsWorkflowsRoute = settingsChild(
  "workflows",
  () => import("@/pages/workflows"),
  "WorkflowsPage",
);
const settingsWorkflowDetailRoute = settingsChild(
  "workflows/$workflowId",
  () => import("@/pages/workflow-detail"),
  "WorkflowDetailPage",
);
const settingsSystemRoute = settingsChild(
  "system",
  () => import("@/components/settings/system-logs-panel"),
  "SystemLogsPanel",
);
const settingsUsersRoute = settingsChild(
  "users",
  () => import("@/pages/users"),
  "UsersPage",
);
const settingsTraktCallbackRoute = settingsChild(
  "trakt/callback",
  () => import("@/pages/trakt-callback"),
  "TraktCallbackPage",
);

// ─── Backward-compatible redirects from old top-level paths ──────────────

function redirectRoute<const F extends string>(from: F, to: string) {
  return createRoute({
    getParentRoute: () => rootRoute,
    path: from,
    beforeLoad: () => {
      throw redirect({ to: to as never, replace: true });
    },
  });
}

const redirects = [
  redirectRoute("/indexers", "/settings/indexers"),
  redirectRoute("/indexers/health", "/settings/health"),
  redirectRoute("/sources", "/settings/sources"),
  redirectRoute("/import-lists", "/settings/import-lists"),
  redirectRoute("/proxies", "/settings/proxies"),
  redirectRoute("/quality-profiles", "/settings/quality-profiles"),
  redirectRoute("/custom-formats", "/settings/custom-formats"),
  redirectRoute("/language-profiles", "/settings/language-profiles"),
  redirectRoute("/notifications", "/settings/notifications"),
  redirectRoute("/events", "/settings/events"),
  redirectRoute("/users", "/settings/users"),
  redirectRoute("/workflows", "/settings/workflows"),
  redirectRoute("/search-queue", "/settings/search-queue"),
];

const workflowDetailRedirectRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/workflows/$workflowId",
  beforeLoad: ({ params }) => {
    throw redirect({
      to: "/settings/workflows/$workflowId",
      params: { workflowId: (params as { workflowId: string }).workflowId },
      replace: true,
    });
  },
});

const inviteRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/invite/$token",
  component: lazyRouteComponent(() => import("@/pages/invite"), "InvitePage"),
  pendingComponent: PageLoader,
  errorComponent: ErrorFallback,
});

const routeTree = rootRoute.addChildren([
  setupRoute,
  indexRoute,
  inviteRoute,
  libraryRoute,
  moviesRoute,
  seriesRoute,
  musicRoute,
  musicArtistRoute,
  discoverRoute,
  requestsRoute,
  analyticsRoute,
  activityRoute,
  calendarRoute,
  downloadsRoute,
  settingsRoute.addChildren([
    settingsIndexRoute,
    settingsGeneralRoute,
    settingsAppearanceRoute,
    settingsFeaturesRoute,
    settingsMediaManagementRoute,
    settingsMediaPreferencesRoute,
    settingsQualityProfilesRoute,
    settingsCustomFormatsRoute,
    settingsMusicRoute,
    settingsLanguageProfilesRoute,
    settingsDownloadClientsRoute,
    settingsDownloadSafetyRoute,
    settingsRollingSearchRoute,
    settingsIndexersRoute,
    settingsSourcesRoute,
    settingsImportListsRoute,
    settingsProxiesRoute,
    settingsSearchQueueRoute,
    settingsNotificationsRoute,
    settingsRequestBotsRoute,
    settingsPluginsRoute,
    settingsConnectRoute,
    settingsSyncProfilesRoute,
    settingsHealthRoute,
    settingsEventsRoute,
    settingsWorkflowsRoute,
    settingsWorkflowDetailRoute,
    settingsSystemRoute,
    settingsUsersRoute,
    settingsTraktCallbackRoute,
  ]),
  ...redirects,
  workflowDetailRedirectRoute,
]);

export const router = createRouter({
  routeTree,
  defaultPreload: "intent",
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
