import { useState, useMemo } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { useSetPageHeader } from "@/hooks/use-page-header";
import { useCalendarEvents, type CalendarEvent } from "@/lib/calendar-api";
import { ChevronLeft, ChevronRight } from "lucide-react";

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

  const { start, end } = useMemo(() => getMonthRange(year, month), [year, month]);
  const { data: events = [], isLoading } = useCalendarEvents(start, end);

  const daysInMonth = getDaysInMonth(year, month);
  const startOffset = getStartDayOffset(year, month);
  const totalCells = Math.ceil((daysInMonth + startOffset) / 7) * 7;

  const eventsByDate = useMemo(() => {
    const map: Record<string, CalendarEvent[]> = {};
    for (const ev of events) {
      if (!map[ev.date]) map[ev.date] = [];
      map[ev.date]!.push(ev);
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
          <Button variant="ghost" size="icon" onClick={prevMonth}>
            <ChevronLeft className="h-4 w-4" />
          </Button>
          <CardTitle>{monthLabel}</CardTitle>
          <Button variant="ghost" size="icon" onClick={nextMonth}>
            <ChevronRight className="h-4 w-4" />
          </Button>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <div className="grid grid-cols-7 gap-px overflow-hidden rounded-md border border-border bg-border" role="status" aria-label="Loading calendar">
              {Array.from({ length: 42 }).map((_, i) => (
                <div key={i} className="min-h-[5rem] bg-card p-1.5">
                  <Skeleton className="h-4 w-4 rounded" />
                </div>
              ))}
            </div>
          ) : (
            <div
              role="grid"
              aria-label={`Calendar for ${monthLabel}`}
              className="grid grid-cols-7 gap-px overflow-hidden rounded-md border border-border bg-border text-sm"
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
                const dayEvents = isValidDay ? eventsByDate[dateStr] ?? [] : [];

                return (
                  <div
                    key={i}
                    role="gridcell"
                    className={`min-h-24 bg-card p-1.5 ${
                      isToday ? "ring-2 ring-inset ring-primary" : ""
                    } ${!isValidDay ? "bg-muted/30" : ""}`}
                  >
                    {isValidDay && (
                      <>
                        <span
                          className={`inline-block mb-1 text-xs font-medium ${
                            isToday
                              ? "rounded-full bg-primary px-1.5 py-0.5 text-primary-foreground"
                              : "text-muted-foreground"
                          }`}
                        >
                          {dayNum}
                        </span>
                        <div className="space-y-0.5">
                          {dayEvents.slice(0, 3).map((ev) => (
                            <EventPill key={ev.id} event={ev} />
                          ))}
                          {dayEvents.length > 3 && (
                            <span className="block text-[10px] text-muted-foreground">
                              +{dayEvents.length - 3} more
                            </span>
                          )}
                        </div>
                      </>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function EventPill({ event }: { event: CalendarEvent }) {
  const isMovie = event.type === "movie";
  const isMissing = event.status === "missing";

  const label = isMovie
    ? event.title
    : `${event.seriesTitle ?? "Unknown"} — S${String(event.season).padStart(2, "0")}E${String(event.episode).padStart(2, "0")}`;

  const tooltip = isMovie
    ? `${event.title}${event.year ? ` (${event.year})` : ""}${event.releaseType && event.releaseType !== "release" ? ` [${event.releaseType}]` : ""}`
    : `${event.seriesTitle ?? "Unknown"} — S${String(event.season).padStart(2, "0")}E${String(event.episode).padStart(2, "0")} — ${event.episodeTitle ?? event.title}`;

  // Color scheme: movies=blue, episodes=purple, theatrical=amber, digital=teal
  const colorMap = {
    movie: isMissing
      ? "border-blue-500/50 text-blue-400"
      : "bg-blue-600 hover:bg-blue-700 text-white",
    theatrical: isMissing
      ? "border-amber-500/50 text-amber-400"
      : "bg-amber-600 hover:bg-amber-700 text-white",
    digital: isMissing
      ? "border-teal-500/50 text-teal-400"
      : "bg-teal-600 hover:bg-teal-700 text-white",
    episode: isMissing
      ? "border-purple-500/50 text-purple-400"
      : "bg-purple-600 hover:bg-purple-700 text-white",
  };

  const movieColorKeys: Record<string, keyof typeof colorMap> = { theatrical: "theatrical", digital: "digital" };
  const colorKey = isMovie
    ? movieColorKeys[event.releaseType ?? ""] ?? "movie"
    : "episode";
  const releaseLabel = event.releaseType === "theatrical" ? "🎬 " : event.releaseType === "digital" ? "💿 " : "";

  return (
    <Badge
      variant={isMissing ? "outline" : "default"}
      className={`block w-full truncate text-[10px] px-1 py-0 font-normal ${isMissing ? "border " : ""}${colorMap[colorKey]}`}
      title={tooltip}
    >
      {releaseLabel}{label}
    </Badge>
  );
}

