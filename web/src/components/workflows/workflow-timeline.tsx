import * as React from "react";
import {
  AlertTriangle,
  Ban,
  CheckCircle2,
  ChevronDown,
  Clock,
  Download,
  Loader2,
  RefreshCw,
  Search,
  XCircle,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { relativeTime } from "@/lib/utils";
import {
  useWorkflowEvents,
  type WorkflowEventType,
  type WorkflowTimelineEvent,
} from "@/lib/workflows-api";
import { LoadingState } from "@/components/ui/loading-state";

// ─── Event type presentation ────────────────────────────────────────────

interface EventConfig {
  icon: React.ElementType;
  color: string; // Tailwind text color
  dotColor: string; // Tailwind bg color for the timeline dot
}

const EVENT_CONFIG: Record<WorkflowEventType, EventConfig> = {
  search_started: {
    icon: Search,
    color: "text-blue-400",
    dotColor: "bg-blue-400",
  },
  grabbed: { icon: Download, color: "text-blue-400", dotColor: "bg-blue-400" },
  downloading: {
    icon: Loader2,
    color: "text-blue-400",
    dotColor: "bg-blue-400",
  },
  download_progress: {
    icon: Loader2,
    color: "text-blue-400",
    dotColor: "bg-blue-400",
  },
  download_complete: {
    icon: CheckCircle2,
    color: "text-green-400",
    dotColor: "bg-green-400",
  },
  import_started: {
    icon: Clock,
    color: "text-blue-400",
    dotColor: "bg-blue-400",
  },
  import_success: {
    icon: CheckCircle2,
    color: "text-green-400",
    dotColor: "bg-green-400",
  },
  import_failed: {
    icon: XCircle,
    color: "text-red-400",
    dotColor: "bg-red-400",
  },
  stale_detected: {
    icon: AlertTriangle,
    color: "text-yellow-400",
    dotColor: "bg-yellow-400",
  },
  retried: {
    icon: RefreshCw,
    color: "text-yellow-400",
    dotColor: "bg-yellow-400",
  },
  failed: { icon: XCircle, color: "text-red-400", dotColor: "bg-red-400" },
  cancelled: { icon: Ban, color: "text-red-400", dotColor: "bg-red-400" },
  completed: {
    icon: CheckCircle2,
    color: "text-green-400",
    dotColor: "bg-green-400",
  },
};

function getEventConfig(eventType: string): EventConfig {
  return (
    EVENT_CONFIG[eventType as WorkflowEventType] ?? {
      icon: Clock,
      color: "text-muted-foreground",
      dotColor: "bg-muted-foreground",
    }
  );
}

// ─── Metadata display ───────────────────────────────────────────────────

function MetadataSection({ metadata }: { metadata: string }) {
  const [open, setOpen] = React.useState(false);

  if (!metadata || metadata === "{}" || metadata === "null") return null;

  let parsed: Record<string, unknown>;
  try {
    parsed = JSON.parse(metadata);
    if (!parsed || Object.keys(parsed).length === 0) return null;
  } catch {
    return null;
  }

  return (
    <div className="mt-1">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="flex items-center gap-1 text-xs text-muted-foreground transition-colors hover:text-foreground"
      >
        <ChevronDown
          className={cn("h-3 w-3 transition-transform", open && "rotate-180")}
        />
        Details
      </button>
      {open && (
        <pre className="mt-1 max-w-md overflow-x-auto rounded bg-muted/50 p-2 text-xs text-muted-foreground">
          {JSON.stringify(parsed, null, 2)}
        </pre>
      )}
    </div>
  );
}

// ─── Single timeline event ──────────────────────────────────────────────

function TimelineItem({
  event,
  isLast,
}: {
  event: WorkflowTimelineEvent;
  isLast: boolean;
}) {
  const config = getEventConfig(event.eventType);
  const Icon = config.icon;

  return (
    <div className="relative flex gap-3">
      {/* Vertical connector line */}
      {!isLast && (
        <div className="absolute bottom-0 left-[11px] top-6 w-px bg-border" />
      )}

      {/* Dot */}
      <div
        className={cn(
          "relative z-10 mt-1 flex h-6 w-6 shrink-0 items-center justify-center rounded-full border border-border bg-background",
        )}
      >
        <div className={cn("h-2.5 w-2.5 rounded-full", config.dotColor)} />
      </div>

      {/* Content */}
      <div className="flex-1 pb-4">
        <div className="flex items-center gap-2">
          <Icon className={cn("h-3.5 w-3.5", config.color)} />
          <span className="text-sm font-medium">{event.message}</span>
        </div>
        <div className="mt-0.5 flex items-center gap-2">
          <span className="text-xs text-muted-foreground">
            {relativeTime(event.createdAt)}
          </span>
          <span className="text-xs text-muted-foreground/50">·</span>
          <span className="text-xs capitalize text-muted-foreground">
            {event.eventType ? event.eventType.replace(/_/g, " ") : ""}
          </span>
        </div>
        <MetadataSection metadata={event.metadata} />
      </div>
    </div>
  );
}

// ─── Main component ─────────────────────────────────────────────────────

export function WorkflowTimeline({ workflowId }: { workflowId: string }) {
  const { data: events, isLoading, error } = useWorkflowEvents(workflowId);

  if (isLoading) return <LoadingState label="Loading events…" />;

  if (error) {
    return (
      <div className="flex items-center gap-2 text-sm text-red-400">
        <AlertTriangle className="h-4 w-4" />
        Failed to load events: {error.message}
      </div>
    );
  }

  const sorted = [...(events ?? [])].sort(
    (a, b) => new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime(),
  );

  if (sorted.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">No events recorded yet.</p>
    );
  }

  return (
    <div className="space-y-0">
      {sorted.map((event, i) => (
        <TimelineItem
          key={event.id}
          event={event}
          isLast={i === sorted.length - 1}
        />
      ))}
    </div>
  );
}
