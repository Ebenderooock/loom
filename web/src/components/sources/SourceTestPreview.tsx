import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { AlertCircle, CheckCircle2, Loader2 } from "lucide-react";
import type { TestSourceResult } from "@/lib/sources-api";

interface SourceTestPreviewProps {
  open: boolean;
  onClose: () => void;
  isLoading?: boolean;
  result?: TestSourceResult;
  error?: Error;
  sourceName?: string;
}

export function SourceTestPreview({
  open,
  onClose,
  isLoading,
  result,
  error,
  sourceName,
}: SourceTestPreviewProps) {
  return (
    <Dialog open={open} onOpenChange={onClose}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>Test Source: {sourceName}</DialogTitle>
          <DialogDescription>
            Preview of items extracted from the source
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {isLoading && (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
              <span className="ml-2 text-muted-foreground">Testing source...</span>
            </div>
          )}

          {error && (
            <Alert variant="destructive">
              <AlertCircle className="h-4 w-4" />
              <AlertTitle>Test Failed</AlertTitle>
              <AlertDescription>{error.message}</AlertDescription>
            </Alert>
          )}

          {result && !result.success && (
            <Alert variant="destructive">
              <AlertCircle className="h-4 w-4" />
              <AlertTitle>Test Failed</AlertTitle>
              <AlertDescription>{result.error}</AlertDescription>
            </Alert>
          )}

          {result && result.success && (
            <>
              <Alert>
                <CheckCircle2 className="h-4 w-4 text-green-600" />
                <AlertTitle className="text-green-600">Test Successful</AlertTitle>
                <AlertDescription>
                  Found {result.items?.length ?? 0} items
                </AlertDescription>
              </Alert>

              {result.items && result.items.length > 0 ? (
                <div className="space-y-3">
                  <p className="text-sm font-medium">Preview (first 5 items):</p>
                  {result.items.map((item, idx) => (
                    <div
                      key={idx}
                      className="border rounded-lg p-4 space-y-2 bg-muted/50"
                    >
                      <div className="flex items-start justify-between gap-2">
                        <div className="flex-1">
                          <p className="font-medium text-sm break-words">{item.title}</p>
                          {item.link && (
                            <a
                              href={item.link}
                              target="_blank"
                              rel="noopener noreferrer"
                              className="text-xs text-blue-600 hover:underline break-all"
                            >
                              {item.link}
                            </a>
                          )}
                        </div>
                        <Badge variant="outline">{idx + 1}</Badge>
                      </div>
                      {item.published && (
                        <p className="text-xs text-muted-foreground">
                          {new Date(item.published).toLocaleString()}
                        </p>
                      )}
                    </div>
                  ))}
                </div>
              ) : (
                <Alert>
                  <AlertCircle className="h-4 w-4" />
                  <AlertTitle>No Items Found</AlertTitle>
                  <AlertDescription>
                    The source returned successfully but no items were extracted. Check your
                    configuration.
                  </AlertDescription>
                </Alert>
              )}
            </>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
