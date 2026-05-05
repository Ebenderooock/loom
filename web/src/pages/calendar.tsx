import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { useSetPageHeader } from "@/hooks/use-page-header";

const DAYS = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];

export function CalendarPage() {
  useSetPageHeader("Calendar");
  const today = new Date();
  const monthLabel = today.toLocaleString(undefined, {
    month: "long",
    year: "numeric",
  });

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>{monthLabel}</CardTitle>
        </CardHeader>
        <CardContent>
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
            {Array.from({ length: 35 }).map((_, i) => {
              const day = i + 1;
              return (
                <div
                  key={i}
                  role="gridcell"
                  className="min-h-20 bg-card p-2 text-xs text-muted-foreground"
                >
                  {day <= 31 ? day : ""}
                </div>
              );
            })}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
