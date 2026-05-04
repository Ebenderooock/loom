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
import { SetupPage } from "@/pages/setup";
import { NotFoundPage } from "@/pages/not-found";
import { useAuth } from "@/hooks/use-auth";

function RootComponent() {
  const { isSetupComplete } = useAuth();

  if (!isSetupComplete) {
    return <SetupPage />;
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

const routeTree = rootRoute.addChildren([
  setupRoute,
  indexRoute,
  libraryRoute,
  moviesRoute,
  activityRoute,
  calendarRoute,
  indexersRoute,
  proxiesRoute,
  downloadsRoute,
  sourcesRoute,
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
