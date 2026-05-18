import * as React from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { LoadingState } from "@/components/ui/loading-state";
import { useSetPageHeader } from "@/hooks/use-page-header";
import { WorkflowTimeline } from "@/components/workflows/workflow-timeline";
import { relativeTime } from "@/lib/utils";
import {
  useWorkflow,
  useCancelWorkflow,
  useRetryWorkflow,
  type WorkflowState,
} from "@/lib/workflows-api";
import {
  AlertTriangle,
  ArrowLeft,
  Ban,
  CheckCircle2,
  Clock,
  Download,
  Loader2,
  RefreshCw,
  Search,
  XCircle,
} from "lucide-react";
import { useRouter, useParams } from "@tanstack/react-router";

// Reuse state badge config from workflows page
const STATE_CONFIG: Record<
  WorkflowState,
  { label: string; icon: React.ElementType; variant: "default" | "secondary" | "destructive" | "outline" }
> = {
  searching: { label: "Searching", icon: Search, variant: "secondary" },
  grabbed: { label: "Grabbed", icon: Download, variant: "secondary" },
  downloading: { label: "Downloading", icon: Loader2, variant: "default" },
  importing: { label: "Importing", icon: Clock, variant: "default" },
  completed: { label: "Completed", icon: CheckCircle2, variant: "outline" },
  failed: { label: "Failed", icon: XCircle, variant: "destructive" },
  cancelled: { label: "Cancelled", icon: Ban, variant: "outline" },
};

const ACTIVE_STATES: WorkflowState[] = ["searching", "grabbed", "downloading", "importing"];

export function WorkflowDetailPage() {
  const { workflowId } = useParams({ strict: false }) as { workflowId: string };
  useSetPageHeader("Workflow Detail", "Event timeline and workflow state");

  const router = useRouter();
  const { data: workflow, isLoading, error } = useWorkflow(workflowId);
  const cancelMut = useCancelWorkflow();
  const retryMut = useRetryWorkflow();

  if (isLoading) return <LoadingState label="Loading workflow…" />;

  if (error || !workflow) {
    return (
      <div className="p-6">
        <Card>
          <CardContent className="py-8 text-center text-red-400">
            <AlertTriangle className="mx-auto mb-2 h-6 w-6" />
            {error ? `Failed to load workflow: ${error.message}` : "Workflow not found"}
          </CardContent>
        </Card>
      </div>
    );
  }

  const config = STATE_CONFIG[workflow.state] ?? STATE_CONFIG.searching;
  const StateIcon = config.icon;
  const isActive = ACTIVE_STATES.includes(workflow.state);
  const isFailed = workflow.state === "failed";

  return (
    <div className="space-y-4 p-6">
      {/* Header */}
      <div className="flex items-center gap-3">
        <Button
          variant="ghost"
          size="icon"
          className="h-8 w-8"
          onClick={() => router.history.back()}
        >
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div className="flex-1 min-w-0">
          <h2 className="text-lg font-semibold truncate">
            {workflow.grabTitle || workflow.id.slice(0, 12)}
          </h2>
          <p className="text-xs text-muted-foreground">
            Started {relativeTime(workflow.createdAt)} · Updated {relativeTime(workflow.updatedAt)}
          </p>
        </div>
        <Badge variant={config.variant} className="gap-1 capitalize">
          <StateIcon className={`h-3 w-3 ${workflow.state === "downloading" ? "animate-spin" : ""}`} />
          {config.label}
        </Badge>
      </div>

      {/* Actions */}
      {(isActive || isFailed) && (
        <div className="flex gap-2">
          {isActive && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => cancelMut.mutate(workflow.id)}
              disabled={cancelMut.isPending}
            >
              <Ban className="mr-1.5 h-3.5 w-3.5" />
              Cancel
            </Button>
          )}
          {isFailed && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => retryMut.mutate(workflow.id)}
              disabled={retryMut.isPending}
            >
              <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
              Retry
            </Button>
          )}
        </div>
      )}

      {/* Error banner */}
      {workflow.lastError && (
        <Card className="border-red-900/50 bg-red-950/20">
          <CardContent className="py-3 text-sm text-red-400">
            <span className="font-medium">Error:</span> {workflow.lastError}
          </CardContent>
        </Card>
      )}

      {/* Timeline */}
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-base">Event Timeline</CardTitle>
        </CardHeader>
        <CardContent>
          <WorkflowTimeline workflowId={workflowId} />
        </CardContent>
      </Card>
    </div>
  );
}
