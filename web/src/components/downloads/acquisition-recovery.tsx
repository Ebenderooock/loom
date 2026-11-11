import { useEffect, useRef, useState } from "react";
import {
  registerAcquisitionRecovery,
  type AcquisitionDecision,
} from "@/lib/fetch";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

interface PendingDecision {
  decision: AcquisitionDecision;
  resolve: (retry: boolean) => void;
}

export function AcquisitionRecovery() {
  const [pending, setPending] = useState<PendingDecision[]>([]);
  const queue = useRef<PendingDecision[]>([]);
  useEffect(() => {
    const unregister = registerAcquisitionRecovery(
      (decision) =>
        new Promise<boolean>((resolve) => {
          queue.current = [...queue.current, { decision, resolve }];
          setPending(queue.current);
        }),
    );
    return () => {
      unregister();
      queue.current.forEach((item) => item.resolve(false));
      queue.current = [];
    };
  }, []);

  const current = pending[0];
  const finish = (retry: boolean) => {
    current?.resolve(retry);
    queue.current = queue.current.slice(1);
    setPending(queue.current);
  };
  return (
    <Dialog
      open={!!current}
      onOpenChange={(open) => {
        if (!open) finish(false);
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            {current?.decision.retry_allowed
              ? "Retry this release?"
              : "Download already in progress"}
          </DialogTitle>
          <DialogDescription>
            {!current?.decision.retry_allowed
              ? "This release is already being sent to a download client. Wait for it to finish before trying again."
              : current.decision.status === "acquiring"
                ? "The previous attempt did not confirm whether the download client accepted this release. Check the client queue before retrying to avoid a duplicate download."
                : "This release was attempted before. If downloading or importing failed, retry it here. Loom will check that the original download is no longer active before trying again."}
          </DialogDescription>
        </DialogHeader>
        <p className="break-words text-sm font-medium">
          {current?.decision.title}
        </p>
        <DialogFooter>
          <Button variant="outline" onClick={() => finish(false)}>
            {current?.decision.retry_allowed ? "Cancel" : "Close"}
          </Button>
          {current?.decision.retry_allowed && (
            <Button onClick={() => finish(true)}>Retry release</Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
