import * as React from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { EmptyState } from "@/components/ui/empty-state";
import { LoadingState } from "@/components/ui/loading-state";
import { useSetPageHeader } from "@/hooks/use-page-header";
import {
  AlertTriangle,
  Ban,
  CheckCircle2,
  Clock,
  Download,
  Loader2,
  MoreHorizontal,
  RefreshCw,
  Search,
  Trash2,
  XCircle,
} from "lucide-react";
import { Link } from "@tanstack/react-router";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { relativeTime } from "@/lib/utils";
import {
  useWorkflows,
  useCancelWorkflow,
  useRetryWorkflow,
  useDeleteWorkflow,
  type Workflow,
  type WorkflowState,
} from "@/lib/workflows-api";

// ─── State presentation helpers ─────────────────────────────────────────

const STATE_CONFIG: Record<
  WorkflowState,
  {
    label: string;
    icon: React.ElementType;
    variant: "default" | "secondary" | "destructive" | "outline";
  }
> = {
  searching: { label: "Searching", icon: Search, variant: "secondary" },
  grabbed: { label: "Grabbed", icon: Download, variant: "secondary" },
  downloading: { label: "Downloading", icon: Loader2, variant: "default" },
  importing: { label: "Importing", icon: Clock, variant: "default" },
  completed: { label: "Completed", icon: CheckCircle2, variant: "outline" },
  failed: { label: "Failed", icon: XCircle, variant: "destructive" },
  cancelled: { label: "Cancelled", icon: Ban, variant: "outline" },
};

function StateBadge({ state }: { state: WorkflowState }) {
  const config = STATE_CONFIG[state] ?? STATE_CONFIG.searching;
  const Icon = config.icon;
  return (
    <Badge variant={config.variant} className="gap-1 capitalize">
      <Icon
        className={`h-3 w-3 ${state === "downloading" ? "animate-spin" : ""}`}
      />
      {config.label}
    </Badge>
  );
}

function TypeLabel({ type }: { type: string }) {
  switch (type) {
    case "movie_search":
      return "Movie Search";
    case "episode_search":
      return "Episode Search";
    case "manual_import":
      return "Manual Import";
    default:
      return type;
  }
}

// ─── Filter tabs ────────────────────────────────────────────────────────

type Filter = "all" | "active" | "completed" | "failed";

const ACTIVE_STATES: WorkflowState[] = [
  "searching",
  "grabbed",
  "downloading",
  "importing",
];

function filterWorkflows(workflows: Workflow[], filter: Filter): Workflow[] {
  switch (filter) {
    case "active":
      return workflows.filter((w) => ACTIVE_STATES.includes(w.state));
    case "completed":
      return workflows.filter((w) => w.state === "completed");
    case "failed":
      return workflows.filter(
        (w) => w.state === "failed" || w.state === "cancelled",
      );
    default:
      return workflows;
  }
}

// ─── Main page ──────────────────────────────────────────────────────────

export function WorkflowsPage() {
  useSetPageHeader("Workflows", "Track search → download → import pipelines");

  const { data: workflows, isLoading, error } = useWorkflows();
  const cancelMut = useCancelWorkflow();
  const retryMut = useRetryWorkflow();
  const deleteMut = useDeleteWorkflow();

  const [filter, setFilter] = React.useState<Filter>("all");
  const [confirmDialog, setConfirmDialog] = React.useState<{
    action: "cancel" | "retry" | "delete";
    workflow: Workflow;
  } | null>(null);

  if (isLoading) return <LoadingState label="Loading workflows…" />;
  if (error) {
    return (
      <div className="p-6">
        <Card>
          <CardContent className="py-8 text-center text-red-400">
            <AlertTriangle className="mx-auto mb-2 h-6 w-6" />
            Failed to load workflows: {error.message}
          </CardContent>
        </Card>
      </div>
    );
  }

  const all = workflows ?? [];
  const filtered = filterWorkflows(all, filter);
  const counts = {
    all: all.length,
    active: all.filter((w) => ACTIVE_STATES.includes(w.state)).length,
    completed: all.filter((w) => w.state === "completed").length,
    failed: all.filter((w) => w.state === "failed" || w.state === "cancelled")
      .length,
  };

  function handleAction(action: "cancel" | "retry" | "delete", wf: Workflow) {
    setConfirmDialog({ action, workflow: wf });
  }

  async function executeAction() {
    if (!confirmDialog) return;
    const { action, workflow } = confirmDialog;
    try {
      if (action === "cancel") await cancelMut.mutateAsync(workflow.id);
      else if (action === "retry") await retryMut.mutateAsync(workflow.id);
      else if (action === "delete") await deleteMut.mutateAsync(workflow.id);
    } finally {
      setConfirmDialog(null);
    }
  }

  const actionLabels = { cancel: "Cancel", retry: "Retry", delete: "Delete" };

  return (
    <div className="space-y-4 p-6">
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-lg">Pipeline Workflows</CardTitle>
        </CardHeader>
        <CardContent>
          <Tabs value={filter} onValueChange={(v) => setFilter(v as Filter)}>
            <TabsList className="mb-4">
              <TabsTrigger value="all">All ({counts.all})</TabsTrigger>
              <TabsTrigger value="active">Active ({counts.active})</TabsTrigger>
              <TabsTrigger value="completed">
                Completed ({counts.completed})
              </TabsTrigger>
              <TabsTrigger value="failed">Failed ({counts.failed})</TabsTrigger>
            </TabsList>

            <TabsContent value={filter} className="mt-0">
              {filtered.length === 0 ? (
                <EmptyState
                  title="No workflows"
                  description={
                    filter === "all"
                      ? "Workflows will appear here when searches or imports are triggered."
                      : `No ${filter} workflows right now.`
                  }
                />
              ) : (
                <div className="overflow-x-auto">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead className="w-[200px]">Title</TableHead>
                        <TableHead>Type</TableHead>
                        <TableHead>State</TableHead>
                        <TableHead>Retries</TableHead>
                        <TableHead>Started</TableHead>
                        <TableHead>Updated</TableHead>
                        <TableHead className="w-[80px]">Error</TableHead>
                        <TableHead className="w-[50px]" />
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {filtered.map((wf) => (
                        <WorkflowRow
                          key={wf.id}
                          workflow={wf}
                          onAction={handleAction}
                        />
                      ))}
                    </TableBody>
                  </Table>
                </div>
              )}
            </TabsContent>
          </Tabs>
        </CardContent>
      </Card>

      {/* Confirm dialog */}
      <Dialog
        open={!!confirmDialog}
        onOpenChange={() => setConfirmDialog(null)}
      >
        {confirmDialog && (
          <DialogContent>
            <DialogHeader>
              <DialogTitle>
                {actionLabels[confirmDialog.action]} Workflow
              </DialogTitle>
              <DialogDescription>
                {confirmDialog.action === "cancel"
                  ? `Cancel the workflow for "${confirmDialog.workflow.grabTitle || confirmDialog.workflow.id}"? This will stop any in-progress operations.`
                  : confirmDialog.action === "retry"
                    ? `Retry the failed workflow for "${confirmDialog.workflow.grabTitle || confirmDialog.workflow.id}"?`
                    : `Delete the workflow record for "${confirmDialog.workflow.grabTitle || confirmDialog.workflow.id}"? This cannot be undone.`}
              </DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button variant="outline" onClick={() => setConfirmDialog(null)}>
                Cancel
              </Button>
              <Button
                variant={
                  confirmDialog.action === "delete" ? "destructive" : "default"
                }
                onClick={executeAction}
                disabled={
                  cancelMut.isPending ||
                  retryMut.isPending ||
                  deleteMut.isPending
                }
              >
                {cancelMut.isPending ||
                retryMut.isPending ||
                deleteMut.isPending ? (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                ) : null}
                {actionLabels[confirmDialog.action]}
              </Button>
            </DialogFooter>
          </DialogContent>
        )}
      </Dialog>
    </div>
  );
}

// ─── Single row ─────────────────────────────────────────────────────────

function WorkflowRow({
  workflow: wf,
  onAction,
}: {
  workflow: Workflow;
  onAction: (action: "cancel" | "retry" | "delete", wf: Workflow) => void;
}) {
  const isActive = ACTIVE_STATES.includes(wf.state);
  const isFailed = wf.state === "failed";
  const isTerminal = wf.state === "completed" || wf.state === "cancelled";

  return (
    <TableRow className={isFailed ? "bg-red-950/10" : undefined}>
      <TableCell
        className="max-w-[200px] truncate font-medium"
        title={wf.grabTitle || wf.id}
      >
        <Link
          to="/workflows/$workflowId"
          params={{ workflowId: wf.id }}
          className="hover:underline"
        >
          {wf.grabTitle || wf.id.slice(0, 8)}
        </Link>
      </TableCell>
      <TableCell className="text-xs text-muted-foreground">
        <TypeLabel type={wf.type} />
      </TableCell>
      <TableCell>
        <StateBadge state={wf.state} />
      </TableCell>
      <TableCell className="text-center">
        {wf.retryCount > 0 ? (
          <span className="text-xs text-amber-400">
            {wf.retryCount}/{wf.maxRetries}
          </span>
        ) : (
          <span className="text-xs text-muted-foreground">—</span>
        )}
      </TableCell>
      <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
        {relativeTime(wf.createdAt)}
      </TableCell>
      <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
        {relativeTime(wf.updatedAt)}
      </TableCell>
      <TableCell>
        {wf.lastError ? (
          <span
            className="block max-w-[200px] cursor-help truncate text-xs text-red-400"
            title={wf.lastError}
          >
            {wf.lastError.length > 40
              ? wf.lastError.slice(0, 40) + "…"
              : wf.lastError}
          </span>
        ) : (
          <span className="text-xs text-muted-foreground">—</span>
        )}
      </TableCell>
      <TableCell>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon" className="h-8 w-8">
              <MoreHorizontal className="h-4 w-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            {isActive && (
              <DropdownMenuItem onClick={() => onAction("cancel", wf)}>
                <Ban className="mr-2 h-4 w-4" />
                Cancel
              </DropdownMenuItem>
            )}
            {isFailed && (
              <DropdownMenuItem onClick={() => onAction("retry", wf)}>
                <RefreshCw className="mr-2 h-4 w-4" />
                Retry
              </DropdownMenuItem>
            )}
            {(isTerminal || isFailed) && (
              <DropdownMenuItem
                className="text-red-400"
                onClick={() => onAction("delete", wf)}
              >
                <Trash2 className="mr-2 h-4 w-4" />
                Delete
              </DropdownMenuItem>
            )}
            {!isActive && !isFailed && !isTerminal && (
              <DropdownMenuItem disabled>No actions</DropdownMenuItem>
            )}
          </DropdownMenuContent>
        </DropdownMenu>
      </TableCell>
    </TableRow>
  );
}
