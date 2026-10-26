import { useState, useMemo } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import {
  LibraryMediaSheet,
  type LibraryMediaTarget,
} from "@/components/library-media-sheet";
import { Skeleton } from "@/components/ui/skeleton";
import { useSetPageHeader } from "@/hooks/use-page-header";
import { useCalendarEvents, type CalendarEvent } from "@/lib/calendar-api";
import {
  ChevronLeft,
  ChevronRight,
  Film,
  Tv,
  Music,
  Check,
  Download,
  Search,
  Clock,
  AlertCircle,
  PackageCheck,
  Bookmark,
  BookmarkX,
} from "lucide-react";
import { cn } from "@/lib/utils";
import {
  episodeReleaseDay,
  formatEpisodeAirDate,
  hasEpisodeAired,
} from "@/lib/episode-airing";
import { useAiringNow } from "@/hooks/use-airing-now";

const DAYS = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];

function formatDate(d: Date): string {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

function getMonthRange(year: number, month: number) {
  const start = new Date(year, month, 1);
  const end = new Date(year, month + 1, 0);
  return { start: formatDate(start), end: formatDate(end) };
}

function getDaysInMonth(year: number, month: number) {
  return new Date(year, month + 1, 0).getDate();
}

// Monday = 0, Tuesday = 1, ..., Sunday = 6
function getStartDayOffset(year: number, month: number) {
  const day = new Date(year, month, 1).getDay();
  return day === 0 ? 6 : day - 1;
}

export function CalendarPage() {
  useSetPageHeader("Calendar");
  const today = new Date();
  const [year, setYear] = useState(today.getFullYear());
  const [month, setMonth] = useState(today.getMonth());
  const [selected, setSelected] = useState<LibraryMediaTarget | null>(null);
  const [expandedDays, setExpandedDays] = useState<Set<string>>(new Set());

  const { start, end } = useMemo(
    () => getMonthRange(year, month),
    [year, month],
  );
  const {
    data: events = [],
    isLoading,
    error,
    refetch,
  } = useCalendarEvents(start, end);
  const airingNow = useAiringNow(events);

  const daysInMonth = getDaysInMonth(year, month);
  const startOffset = getStartDayOffset(year, month);
  const totalCells = Math.ceil((daysInMonth + startOffset) / 7) * 7;

  const eventsByDate = useMemo(() => {
    const map: Record<string, CalendarEvent[]> = {};
    for (const ev of events) {
      const day = episodeReleaseDay({
        airDate: ev.date,
        airDateUtc: ev.airDateUtc,
      });
      if (!map[day]) map[day] = [];
      map[day]!.push(ev);
    }
    return map;
  }, [events]);

  const todayStr = formatDate(today);
  const monthLabel = new Date(year, month).toLocaleString(undefined, {
    month: "long",
    year: "numeric",
  });

  function prevMonth() {
    if (month === 0) {
      setYear(year - 1);
      setMonth(11);
    } else {
      setMonth(month - 1);
    }
  }

  function nextMonth() {
    if (month === 11) {
      setYear(year + 1);
      setMonth(0);
    } else {
      setMonth(month + 1);
    }
  }

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-4">
          <Button
            variant="ghost"
            size="icon"
            onClick={prevMonth}
            aria-label="Previous month"
          >
            <ChevronLeft className="h-4 w-4" />
          </Button>
          <CardTitle>{monthLabel}</CardTitle>
          <Button
            variant="ghost"
            size="icon"
            onClick={nextMonth}
            aria-label="Next month"
          >
            <ChevronRight className="h-4 w-4" />
          </Button>
        </CardHeader>
        <CardContent>
          <p className="mb-3 text-xs text-muted-foreground">
            Select a movie or episode to view details and search for downloads.
          </p>
          <p className="mb-3 text-xs text-muted-foreground sm:hidden">
            Scroll sideways to see the full month.
          </p>
          {error ? (
            <div role="alert" className="space-y-3">
              <p className="text-sm text-destructive">{error.message}</p>
              <Button variant="outline" onClick={() => void refetch()}>
                Retry
              </Button>
            </div>
          ) : isLoading ? (
            <div
              className="grid grid-cols-7 gap-px overflow-hidden rounded-md border border-border bg-border"
              role="status"
              aria-label="Loading calendar"
            >
              {Array.from({ length: 42 }).map((_, i) => (
                <div key={i} className="min-h-[5rem] bg-card p-1.5">
                  <Skeleton className="h-4 w-4 rounded" />
                </div>
              ))}
            </div>
          ) : (
            <div
              className="overflow-x-auto rounded-md"
              role="region"
              aria-label="Scrollable calendar"
            >
              <div
                role="grid"
                aria-label={`Calendar for ${monthLabel}`}
                className="grid min-w-[56rem] grid-cols-7 gap-px overflow-hidden rounded-md border border-border bg-border text-sm"
              >
                {DAYS.map((d) => (
                  <div
                    key={d}
                    role="columnheader"
                    className="bg-muted px-2 py-1 text-center text-xs font-medium text-muted-foreground"
                  >
                    {d}
                  </div>
                ))}
                {Array.from({ length: totalCells }).map((_, i) => {
                  const dayNum = i - startOffset + 1;
                  const isValidDay = dayNum >= 1 && dayNum <= daysInMonth;
                  const dateStr = isValidDay
                    ? `${year}-${String(month + 1).padStart(2, "0")}-${String(dayNum).padStart(2, "0")}`
                    : "";
                  const isToday = dateStr === todayStr;
                  const dayEvents = isValidDay
                    ? (eventsByDate[dateStr] ?? [])
                    : [];

                  return (
                    <div
                      key={i}
                      role="gridcell"
                      className={`min-h-24 min-w-0 bg-card p-1.5 ${
                        isToday ? "ring-2 ring-inset ring-primary" : ""
                      } ${!isValidDay ? "bg-muted/30" : ""}`}
                    >
                      {isValidDay && (
                        <>
                          <span
                            className={`mb-1 inline-block text-xs font-medium ${
                              isToday
                                ? "rounded-full bg-primary px-1.5 py-0.5 text-primary-foreground"
                                : "text-muted-foreground"
                            }`}
                          >
                            {dayNum}
                          </span>
                          <div className="space-y-0.5">
                            {(expandedDays.has(dateStr)
                              ? dayEvents
                              : dayEvents.slice(0, 3)
                            ).map((ev) => (
                              <EventPill
                                key={`${ev.type}-${ev.id}-${ev.releaseType ?? ""}`}
                                event={ev}
                                now={airingNow}
                                onSelect={() => {
                                  if (ev.type === "movie")
                                    setSelected({ type: "movie", id: ev.id });
                                  else if (ev.seriesId)
                                    setSelected({
                                      type: "series",
                                      id: ev.seriesId,
                                    });
                                }}
                              />
                            ))}
                            {dayEvents.length > 3 && (
                              <button
                                type="button"
                                aria-expanded={expandedDays.has(dateStr)}
                                className="min-h-9 w-full rounded px-1 text-left text-xs text-muted-foreground hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                                onClick={() =>
                                  setExpandedDays((prev) => {
                                    const next = new Set(prev);
                                    if (next.has(dateStr)) next.delete(dateStr);
                                    else next.add(dateStr);
                                    return next;
                                  })
                                }
                              >
                                {expandedDays.has(dateStr)
                                  ? "Show less"
                                  : `+${dayEvents.length - 3} more`}
                              </button>
                            )}
                          </div>
                        </>
                      )}
                    </div>
                  );
                })}
              </div>
            </div>
          )}
        </CardContent>
      </Card>
      {selected && (
        <LibraryMediaSheet
          key={`${selected.type}-${selected.id}`}
          target={selected}
          onClose={() => setSelected(null)}
        />
      )}
    </div>
  );
}

function EventPill({
  event,
  now,
  onSelect,
}: {
  event: CalendarEvent;
  now: Date;
  onSelect: () => void;
}) {
  const isMovie = event.type === "movie";
  const isAlbum = event.type === "album";
  const status =
    event.type === "episode" &&
    (event.status === "missing" || event.status === "unaired")
      ? hasEpisodeAired(
          { airDate: event.date, airDateUtc: event.airDateUtc },
          now,
        )
        ? "missing"
        : "unaired"
      : event.status;

  const label =
    isMovie || isAlbum
      ? event.title
      : `${event.seriesTitle ?? "Unknown"} — S${String(event.season).padStart(2, "0")}E${String(event.episode).padStart(2, "0")}`;

  const tooltip = isMovie
    ? `${event.title}${event.year ? ` (${event.year})` : ""}${event.releaseType && event.releaseType !== "release" ? ` [${event.releaseType}]` : ""}`
    : isAlbum
      ? event.title
      : `${event.seriesTitle ?? "Unknown"} — S${String(event.season).padStart(2, "0")}E${String(event.episode).padStart(2, "0")} — ${event.episodeTitle ?? event.title}`;

  const statusMap = {
    downloaded: {
      label: "Available",
      icon: Check,
      color: "text-emerald-600 dark:text-emerald-400",
    },
    missing: {
      label: "Missing",
      icon: AlertCircle,
      color: "text-red-600 dark:text-red-400",
    },
    unaired: { label: "Unaired", icon: Clock, color: "text-muted-foreground" },
    unreleased: {
      label: "Unreleased",
      icon: Clock,
      color: "text-muted-foreground",
    },
    searching: {
      label: "Searching",
      icon: Search,
      color: "text-blue-600 dark:text-blue-400",
    },
    grabbed: {
      label: "Grabbed",
      icon: Download,
      color: "text-amber-600 dark:text-amber-400",
    },
    downloading: {
      label: "Downloading",
      icon: Download,
      color: "text-amber-600 dark:text-amber-400",
    },
    importing: {
      label: "Importing",
      icon: PackageCheck,
      color: "text-blue-600 dark:text-blue-400",
    },
  };
  const state = statusMap[status];
  const MediaIcon = isMovie ? Film : isAlbum ? Music : Tv;
  const MonitorIcon = event.monitored === false ? BookmarkX : Bookmark;
  const monitoringLabel =
    event.monitored === false
      ? "Unmonitored"
      : event.monitored === true
        ? "Monitored"
        : "";
  const description = `${tooltip} — ${state.label}${monitoringLabel ? ` — ${monitoringLabel}` : ""}${event.airDateUtc ? ` — ${formatEpisodeAirDate({ airDateUtc: event.airDateUtc })} (TVmaze)` : ""}`;
  const content = (
    <>
      <span className="flex min-w-0 items-center gap-1">
        <MediaIcon
          aria-hidden="true"
          className="hidden h-3 w-3 shrink-0 sm:block"
        />
        <span className="truncate">{label}</span>
      </span>
      <span
        className={cn(
          "flex flex-wrap items-center gap-x-1 text-[10px]",
          state.color,
        )}
      >
        <state.icon aria-hidden="true" className="h-3 w-3 shrink-0" />
        <span>{state.label}</span>
        {monitoringLabel && (
          <span className="flex items-center gap-0.5 text-muted-foreground">
            <MonitorIcon aria-hidden="true" className="h-3 w-3" />
            <span>{monitoringLabel}</span>
          </span>
        )}
        {isMovie && event.releaseType && event.releaseType !== "release" && (
          <span className="text-muted-foreground">
            {event.releaseType === "theatrical" ? "Theatrical" : "Digital"}
          </span>
        )}
      </span>
    </>
  );
  const className = cn(
    "block min-h-11 w-full min-w-0 rounded border px-1.5 py-1 text-left text-xs",
    isMovie
      ? "border-blue-500/30 bg-blue-500/10"
      : isAlbum
        ? "border-pink-500/30 bg-pink-500/10"
        : "border-purple-500/30 bg-purple-500/10",
  );

  return isMovie || (event.type === "episode" && event.seriesId) ? (
    <button
      type="button"
      onClick={onSelect}
      className={cn(
        className,
        "cursor-pointer transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
      )}
      title={description}
      aria-label={description}
    >
      {content}
    </button>
  ) : (
    <div className={className} title={description}>
      {content}
    </div>
  );
}
