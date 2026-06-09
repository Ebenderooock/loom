import { useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Table,
  TableHeader,
  TableBody,
  TableHead,
  TableRow,
  TableCell,
} from "@/components/ui/table";
import { Loader2, Download, AlertCircle } from "lucide-react";
import { toast } from "sonner";
import { formatBytes } from "@/lib/utils";
import {
  useAlbumReleases,
  useGrabAlbumRelease,
  type ReleaseCandidate,
} from "@/lib/music-api";

interface AlbumSearchDialogProps {
  albumId: string;
  albumTitle: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function AlbumSearchDialog({
  albumId,
  albumTitle,
  open,
  onOpenChange,
}: AlbumSearchDialogProps) {
  const { data, isLoading, isError, error } = useAlbumReleases(albumId, open);
  const grab = useGrabAlbumRelease();
  const [grabbing, setGrabbing] = useState<string | null>(null);

  const handleGrab = async (release: ReleaseCandidate) => {
    setGrabbing(release.guid || release.title);
    try {
      const res = await grab.mutateAsync({ id: albumId, release });
      toast.success(`Grabbed “${res.title}”`);
      onOpenChange(false);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Grab failed");
    } finally {
      setGrabbing(null);
    }
  };

  const releases = data ?? [];

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>Interactive search</DialogTitle>
          <DialogDescription>
            Releases found for “{albumTitle}”. Choose one to send to your
            download client.
          </DialogDescription>
        </DialogHeader>

        {isLoading ? (
          <div className="flex items-center justify-center gap-2 py-12 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" />
            Searching indexers…
          </div>
        ) : isError ? (
          <div className="flex items-center justify-center gap-2 py-12 text-sm text-destructive">
            <AlertCircle className="h-4 w-4" />
            {error instanceof Error ? error.message : "Search failed"}
          </div>
        ) : releases.length === 0 ? (
          <p className="py-12 text-center text-sm text-muted-foreground">
            No matching releases found.
          </p>
        ) : (
          <ScrollArea className="max-h-[60vh]">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Release</TableHead>
                  <TableHead className="w-24">Quality</TableHead>
                  <TableHead className="w-20 text-right">Size</TableHead>
                  <TableHead className="w-16 text-right">Seed</TableHead>
                  <TableHead className="w-16" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {releases.map((r) => {
                  const id = r.guid || r.title;
                  return (
                    <TableRow key={id} className={r.allowed ? "" : "opacity-50"}>
                      <TableCell className="max-w-[320px]">
                        <div className="truncate text-xs font-medium" title={r.title}>
                          {r.title}
                        </div>
                        <div className="flex items-center gap-1.5 text-[10px] text-muted-foreground">
                          <span>{r.indexer_id}</span>
                          <span>· {r.protocol}</span>
                          {!r.allowed && (
                            <Badge variant="outline" className="text-[9px]">
                              rejected
                            </Badge>
                          )}
                          {r.meets_cutoff && (
                            <Badge variant="secondary" className="text-[9px]">
                              cutoff met
                            </Badge>
                          )}
                        </div>
                      </TableCell>
                      <TableCell className="text-xs">
                        {r.quality_name || "—"}
                        {!!r.format_score && (
                          <Badge
                            variant={r.format_score > 0 ? "secondary" : "destructive"}
                            className="ml-1 text-[9px]"
                          >
                            {r.format_score > 0 ? `+${r.format_score}` : r.format_score}
                          </Badge>
                        )}
                      </TableCell>
                      <TableCell className="text-right text-xs">
                        {formatBytes(r.size)}
                      </TableCell>
                      <TableCell className="text-right text-xs">
                        {r.seeders ?? "—"}
                      </TableCell>
                      <TableCell>
                        <Button
                          size="sm"
                          variant="ghost"
                          disabled={grabbing !== null}
                          onClick={() => handleGrab(r)}
                          aria-label="Grab release"
                        >
                          {grabbing === id ? (
                            <Loader2 className="h-4 w-4 animate-spin" />
                          ) : (
                            <Download className="h-4 w-4" />
                          )}
                        </Button>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </ScrollArea>
        )}
      </DialogContent>
    </Dialog>
  );
}
