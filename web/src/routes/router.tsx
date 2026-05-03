import {
  createRootRoute,
  createRoute,
  createRouter,
} from "@tanstack/react-router";
import { AppLayout } from "@/components/layout/app-layout";
import { DashboardPage } from "@/pages/dashboard";
import { LibraryPage } from "@/pages/library";
import { ActivityPage } from "@/pages/activity";
import { CalendarPage } from "@/pages/calendar";
import { SettingsPage } from "@/pages/settings";
import { IndexersPage } from "@/pages/indexers";
import { ProxiesPage } from "@/pages/proxies";
import { NotFoundPage } from "@/pages/not-found";

const rootRoute = createRootRoute({
  component: AppLayout,
  notFoundComponent: NotFoundPage,
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

const routeTree = rootRoute.addChildren([
  indexRoute,
  libraryRoute,
  activityRoute,
  calendarRoute,
  indexersRoute,
  proxiesRoute,
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
