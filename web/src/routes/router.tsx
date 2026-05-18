import { Suspense, lazy } from "react";
import {
  createRootRoute,
  createRoute,
  createRouter,
  lazyRouteComponent,
} from "@tanstack/react-router";
import { AppLayout } from "@/components/layout/app-layout";
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
    return <div className="w-screen h-screen bg-neutral-dark" />;
  }

  if (!isSetupComplete) {
    return (
      <Suspense fallback={<PageLoader />}>
        <SetupPage />
      </Suspense>
    );
  }

  if (!isAuthenticated) {
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
  component: lazyRouteComponent(
    () => import("@/pages/library"),
    "LibraryPage",
  ),
  pendingComponent: PageLoader,
  errorComponent: ErrorFallback,
});

const moviesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/movies",
  component: lazyRouteComponent(() => import("@/pages/movies"), "MoviesPage"),
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

const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/settings",
  component: lazyRouteComponent(
    () => import("@/pages/settings"),
    "SettingsPage",
  ),
  pendingComponent: PageLoader,
  errorComponent: ErrorFallback,
  validateSearch: (search: Record<string, unknown>) => ({
    trakt_code: (search.trakt_code as string) ?? undefined,
  }),
});

const traktCallbackRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/settings/trakt/callback",
  component: lazyRouteComponent(
    () => import("@/pages/trakt-callback"),
    "TraktCallbackPage",
  ),
  pendingComponent: PageLoader,
  errorComponent: ErrorFallback,
});

const indexersRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/indexers",
  component: lazyRouteComponent(
    () => import("@/pages/indexers"),
    "IndexersPage",
  ),
  pendingComponent: PageLoader,
  errorComponent: ErrorFallback,
});

const proxiesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/proxies",
  component: lazyRouteComponent(
    () => import("@/pages/proxies"),
    "ProxiesPage",
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

const sourcesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/sources",
  component: lazyRouteComponent(
    () => import("@/pages/sources"),
    "SourcesPage",
  ),
  pendingComponent: PageLoader,
  errorComponent: ErrorFallback,
});

const seriesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/series",
  component: lazyRouteComponent(() => import("@/pages/series"), "SeriesPage"),
  pendingComponent: PageLoader,
  errorComponent: ErrorFallback,
});

const notificationsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/notifications",
  component: lazyRouteComponent(
    () => import("@/pages/notifications"),
    "NotificationsPage",
  ),
  pendingComponent: PageLoader,
  errorComponent: ErrorFallback,
});

const languageProfilesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/language-profiles",
  component: lazyRouteComponent(
    () => import("@/pages/language-profiles"),
    "LanguageProfilesPage",
  ),
  pendingComponent: PageLoader,
  errorComponent: ErrorFallback,
});

const indexerHealthRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/indexers/health",
  component: lazyRouteComponent(
    () => import("@/pages/indexer-health"),
    "IndexerHealthPage",
  ),
  pendingComponent: PageLoader,
  errorComponent: ErrorFallback,
});

const customFormatsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/custom-formats",
  component: lazyRouteComponent(
    () => import("@/pages/custom-formats"),
    "CustomFormatsPage",
  ),
  pendingComponent: PageLoader,
  errorComponent: ErrorFallback,
});

const qualityProfilesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/quality-profiles",
  component: lazyRouteComponent(
    () => import("@/pages/quality-profiles"),
    "QualityProfilesPage",
  ),
  pendingComponent: PageLoader,
  errorComponent: ErrorFallback,
});

const importListsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/import-lists",
  component: lazyRouteComponent(
    () => import("@/pages/import-lists"),
    "ImportListsPage",
  ),
  pendingComponent: PageLoader,
  errorComponent: ErrorFallback,
});

const eventsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/events",
  component: lazyRouteComponent(
    () => import("@/pages/events"),
    "EventsPage",
  ),
  pendingComponent: PageLoader,
  errorComponent: ErrorFallback,
});

const workflowsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/workflows",
  component: lazyRouteComponent(
    () => import("@/pages/workflows"),
    "WorkflowsPage",
  ),
  pendingComponent: PageLoader,
  errorComponent: ErrorFallback,
});

const workflowDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/workflows/$workflowId",
  component: lazyRouteComponent(
    () => import("@/pages/workflow-detail"),
    "WorkflowDetailPage",
  ),
  pendingComponent: PageLoader,
  errorComponent: ErrorFallback,
});

const routeTree = rootRoute.addChildren([
  setupRoute,
  indexRoute,
  libraryRoute,
  moviesRoute,
  seriesRoute,
  activityRoute,
  calendarRoute,
  indexerHealthRoute,
  indexersRoute,
  proxiesRoute,
  downloadsRoute,
  sourcesRoute,
  notificationsRoute,
  languageProfilesRoute,
  customFormatsRoute,
  qualityProfilesRoute,
  importListsRoute,
  eventsRoute,
  workflowsRoute,
  workflowDetailRoute,
  traktCallbackRoute,
  settingsRoute,
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
