import {
  createRootRoute,
  createRoute,
  createRouter,
} from "@tanstack/react-router";
import { AppLayout } from "@/components/layout/app-layout";
import { DashboardPage } from "@/pages/dashboard";
import { LibraryPage } from "@/pages/library";
import { MoviesPage } from "@/pages/movies";
import { ActivityPage } from "@/pages/activity";
import { CalendarPage } from "@/pages/calendar";
import { SettingsPage } from "@/pages/settings";
import { IndexersPage } from "@/pages/indexers";
import { ProxiesPage } from "@/pages/proxies";
import { DownloadsPage } from "@/pages/downloads";
import { SourcesPage } from "@/pages/sources";
import { SeriesPage } from "@/pages/series";
import { SetupPage } from "@/pages/setup";
import { NotificationsPage } from "@/pages/notifications";
import { LanguageProfilesPage } from "@/pages/language-profiles";
import { IndexerHealthPage } from "@/pages/indexer-health";
import { CustomFormatsPage } from "@/pages/custom-formats";
import { NotFoundPage } from "@/pages/not-found";
import { AuthPage } from "@/pages/auth";
import { useAuth } from "@/hooks/use-auth";

function RootComponent() {
  const { isSetupComplete, isAuthenticated, isLoading } = useAuth();

  if (isLoading) {
    return <div className="w-screen h-screen bg-neutral-dark" />;
  }

  // If setup not complete, show setup flow
  if (!isSetupComplete) {
    return <SetupPage />;
  }

  // If setup complete but not authenticated, show login
  if (!isAuthenticated) {
    return <AuthPage />;
  }

  return <AppLayout />;
}

const rootRoute = createRootRoute({
  component: RootComponent,
  notFoundComponent: NotFoundPage,
});

const setupRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/setup",
  component: SetupPage,
});

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: DashboardPage,
});

const libraryRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/library",
  component: LibraryPage,
});

const moviesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/movies",
  component: MoviesPage,
});

const activityRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/activity",
  component: ActivityPage,
});

const calendarRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/calendar",
  component: CalendarPage,
});

const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/settings",
  component: SettingsPage,
});

const indexersRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/indexers",
  component: IndexersPage,
});

const proxiesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/proxies",
  component: ProxiesPage,
});

const downloadsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/downloads",
  component: DownloadsPage,
});

const sourcesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/sources",
  component: SourcesPage,
});

const seriesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/series",
  component: SeriesPage,
});

const notificationsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/notifications",
  component: NotificationsPage,
});

const languageProfilesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/language-profiles",
  component: LanguageProfilesPage,
});

const indexerHealthRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/indexers/health",
  component: IndexerHealthPage,
});

const customFormatsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/custom-formats",
  component: CustomFormatsPage,
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
