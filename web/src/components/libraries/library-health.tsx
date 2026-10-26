import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ShieldCheck } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import {
  dismissLibraryHealth,
  getLibraryHealth,
  runLibraryHealth,
  type HealthFinding,
  type Library,
} from "@/lib/libraries-api";

const labels: Record<HealthFinding["kind"], string> = {
  sample: "Suspected sample",
  duplicate: "Possible duplicate",
  orphan_record: "Orphaned record",
  untracked_file: "Untracked file",
  missing_file: "Missing / not observed",
  protected: "Protected item",
};
const pageSize = 25;

export function LibraryHealth({
  library,
}: {
  library: Pick<Library, "id" | "name">;
}) {
  const [open, setOpen] = useState(false);
  const [running, setRunning] = useState(false);
  const [dismissBusy, setDismissBusy] = useState<string | null>(null);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [showDismissed, setShowDismissed] = useState(false);
  const [page, setPage] = useState(0);
  const controller = useRef<AbortController | null>(null);
  const qc = useQueryClient();
  const queryKey = ["libraries", library.id, "health"];
  const query = useQuery({
    queryKey,
    queryFn: ({ signal }) => getLibraryHealth(library.id, signal),
    enabled: open,
    retry: false,
  });

  useEffect(() => () => controller.current?.abort(), []);

  const changeOpen = (next: boolean) => {
    if (!next) controller.current?.abort();
    setOpen(next);
  };
  const run = async () => {
    const abort = new AbortController();
    controller.current = abort;
    setRunning(true);
    setError("");
    setMessage("");
    try {
      const report = await runLibraryHealth(library.id, abort.signal);
      qc.setQueryData(queryKey, report);
      setPage(0);
    } catch (err) {
      if (abort.signal.aborted) {
        setMessage(
          "Check cancelled. Reload the report to retrieve any saved partial snapshot; an older report is not a new result.",
        );
      } else {
        setError(
          err instanceof Error
            ? err.message
            : "Health check failed. Retry after checking server availability.",
        );
      }
    } finally {
      controller.current = null;
      setRunning(false);
    }
  };
  const dismiss = async (finding: HealthFinding) => {
    setDismissBusy(finding.id);
    setError("");
    try {
      const report = await dismissLibraryHealth(
        library.id,
        finding.id,
        !finding.dismissed,
      );
      qc.setQueryData(queryKey, report);
      setPage(0);
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : "Dismissal failed. Reload the report and retry.",
      );
    } finally {
      setDismissBusy(null);
    }
  };
  const report = query.data;
  const findings =
    report?.findings.filter((finding) => showDismissed || !finding.dismissed) ??
    [];
  const pageCount = Math.max(1, Math.ceil(findings.length / pageSize));
  const currentPage = Math.min(page, pageCount - 1);

  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          className="min-h-11 shrink-0"
          aria-label={`Library health for ${library.name}`}
        >
          <ShieldCheck className="mr-2 h-4 w-4" aria-hidden="true" /> Health
        </Button>
      </DialogTrigger>
      <DialogContent className="max-w-4xl">
        <DialogHeader>
          <DialogTitle>Library health: {library.name}</DialogTitle>
          <DialogDescription>
            Read-only evidence, not a cleanup command. No files, media records
            or monitoring state are changed. Filename and metadata heuristics
            cannot prove content equality or duration.
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-wrap items-center gap-2">
          <Button
            className="min-h-11"
            onClick={run}
            disabled={running || dismissBusy !== null}
          >
            {running
              ? "Checking (30-second budget)..."
              : report
                ? "Recheck library"
                : "Run health check"}
          </Button>
          {running && (
            <Button
              className="min-h-11"
              variant="outline"
              onClick={() => controller.current?.abort()}
            >
              Cancel check
            </Button>
          )}
          <Button
            className="min-h-11"
            variant="outline"
            disabled={running || query.isFetching || dismissBusy !== null}
            onClick={() => query.refetch()}
          >
            Reload report
          </Button>
        </div>
        {running && (
          <p role="status" className="text-sm text-muted-foreground">
            Checking a bounded snapshot. Any report below is from the previous
            check.
          </p>
        )}
        {message && (
          <p role="status" className="text-sm">
            {message}
          </p>
        )}
        {(error || query.error) && (
          <p role="alert" className="break-words text-sm text-destructive">
            {error || query.error?.message}
          </p>
        )}
        {query.isPending && query.isFetching && (
          <p role="status">Loading latest report...</p>
        )}
        {query.isSuccess && !report && (
          <p className="text-sm text-muted-foreground">
            No health report yet. Run a check to collect evidence; this does not
            mean the library is healthy.
          </p>
        )}
        {report && (
          <>
            <section
              className="space-y-2 rounded-lg border border-border bg-muted/30 p-4"
              aria-label="Report coverage"
            >
              <div className="flex flex-wrap items-center gap-2">
                <Badge
                  variant={
                    report.status === "complete" ? "secondary" : "destructive"
                  }
                >
                  {report.status}
                </Badge>
                <span className="text-sm">
                  {new Date(report.checked_at).toLocaleString()}
                </span>
              </div>
              <p className="break-all font-mono text-xs">{report.root}</p>
              <p className="text-sm text-muted-foreground">
                {report.scanned_entries.toLocaleString()} entries observed
                (limit {report.entry_limit.toLocaleString()});{" "}
                {report.inspected_records.toLocaleString()} records inspected.{" "}
                Complete describes traversal coverage, not confirmed deletions
                or safety to clean up.
              </p>
              <ul className="list-disc space-y-1 pl-5 text-sm">
                {report.warnings.map((warning, i) => (
                  <li className="break-words" key={i}>
                    {warning}
                  </li>
                ))}
              </ul>
            </section>
            <div className="flex flex-wrap items-center justify-between gap-3">
              <p className="text-sm font-medium">
                {findings.length} findings shown
              </p>
              <label className="flex min-h-11 cursor-pointer items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={showDismissed}
                  onChange={(event) => {
                    setShowDismissed(event.target.checked);
                    setPage(0);
                  }}
                />
                Show dismissed findings
              </label>
            </div>
            {findings.length === 0 && (
              <p className="text-sm text-muted-foreground">
                No visible findings in this snapshot. Review coverage warnings;
                absence of findings is not proof of a healthy mount.
              </p>
            )}
            <div className="space-y-3">
              {findings
                .slice(currentPage * pageSize, (currentPage + 1) * pageSize)
                .map((finding) => (
                  <article
                    key={finding.id}
                    className="space-y-3 rounded-lg border border-border p-4"
                  >
                    <div className="flex flex-wrap items-start justify-between gap-2">
                      <div className="space-y-1">
                        <h3 className="font-medium">{labels[finding.kind]}</h3>
                        <p className="text-xs text-muted-foreground">
                          {finding.confidence} confidence
                          {finding.dismissed ? " - dismissed" : ""}
                        </p>
                      </div>
                      <Button
                        variant="outline"
                        className="min-h-11"
                        disabled={running || dismissBusy !== null}
                        onClick={() => dismiss(finding)}
                      >
                        {dismissBusy === finding.id
                          ? "Saving..."
                          : finding.dismissed
                            ? "Restore finding"
                            : "Dismiss finding"}
                      </Button>
                    </div>
                    <p className="break-all font-mono text-xs">
                      {finding.path}
                    </p>
                    {finding.related_paths.length > 0 && (
                      <div className="space-y-1 text-xs">
                        <p className="font-medium">Related paths</p>
                        {finding.related_paths.map((path) => (
                          <p className="break-all font-mono" key={path}>
                            {path}
                          </p>
                        ))}
                      </div>
                    )}
                    <ul className="list-disc space-y-1 pl-5 text-sm">
                      {finding.evidence.map((evidence, i) => (
                        <li className="break-words" key={i}>
                          {evidence}
                        </li>
                      ))}
                    </ul>
                    <details className="rounded-md bg-muted/40 p-3 text-sm">
                      <summary className="min-h-8 cursor-pointer font-medium">
                        Action preview (read-only)
                      </summary>
                      <p className="mt-2 break-words">
                        {finding.preview.outcome}
                      </p>
                      <p className="mt-1 text-xs text-muted-foreground">
                        Suggested review: {finding.preview.action}. Execution is
                        not available.
                      </p>
                    </details>
                  </article>
                ))}
            </div>
            {pageCount > 1 && (
              <nav
                aria-label="Finding pages"
                className="flex items-center justify-between gap-2"
              >
                <Button
                  variant="outline"
                  className="min-h-11"
                  disabled={currentPage === 0}
                  onClick={() => setPage(currentPage - 1)}
                >
                  Previous
                </Button>
                <span className="text-sm">
                  Page {currentPage + 1} of {pageCount}
                </span>
                <Button
                  variant="outline"
                  className="min-h-11"
                  disabled={currentPage + 1 >= pageCount}
                  onClick={() => setPage(currentPage + 1)}
                >
                  Next
                </Button>
              </nav>
            )}
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
