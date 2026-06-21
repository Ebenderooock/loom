import * as React from "react";
import { AlertTriangle } from "lucide-react";
import { Button, type ButtonProps } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

interface ConfirmActionProps {
  actionLabel: string;
  title: string;
  description: string;
  confirmLabel?: string;
  cancelLabel?: string;
  onConfirm: () => Promise<void> | void;
  disabled?: boolean;
  pending?: boolean;
  triggerVariant?: ButtonProps["variant"];
  confirmVariant?: ButtonProps["variant"];
  triggerSize?: ButtonProps["size"];
  icon?: React.ReactNode;
  details?: React.ReactNode;
  className?: string;
}

export function ConfirmActionButton({
  actionLabel,
  title,
  description,
  confirmLabel = "Confirm",
  cancelLabel = "Cancel",
  onConfirm,
  disabled = false,
  pending = false,
  triggerVariant = "destructive",
  confirmVariant = "destructive",
  triggerSize = "sm",
  icon,
  details,
  className,
}: ConfirmActionProps) {
  const [open, setOpen] = React.useState(false);
  const [submitting, setSubmitting] = React.useState(false);

  const handleConfirm = async () => {
    setSubmitting(true);
    try {
      await onConfirm();
      setOpen(false);
    } finally {
      setSubmitting(false);
    }
  };

  const busy = pending || submitting;

  return (
    <>
      <Button
        type="button"
        variant={triggerVariant}
        size={triggerSize}
        className={className}
        disabled={disabled || busy}
        onClick={() => setOpen(true)}
      >
        {icon}
        {actionLabel}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <AlertTriangle className="h-5 w-5 text-destructive" />
              {title}
            </DialogTitle>
            <DialogDescription>{description}</DialogDescription>
          </DialogHeader>
          <div className="rounded-lg border border-destructive/20 bg-destructive/5 p-3 text-sm text-muted-foreground">
            This action clears stored history immediately and cannot be undone.
          </div>
          {details}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setOpen(false)}
              disabled={busy}
            >
              {cancelLabel}
            </Button>
            <Button
              type="button"
              variant={confirmVariant}
              onClick={() => void handleConfirm()}
              disabled={busy}
            >
              {busy ? "Working…" : confirmLabel}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
