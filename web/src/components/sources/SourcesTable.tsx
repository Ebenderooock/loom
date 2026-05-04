import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CheckCircle2, XCircle, Trash2, Edit2 } from "lucide-react";
import type { UserSource } from "@/lib/sources-api";

interface SourcesTableProps {
  sources: UserSource[];
  isLoading: boolean;
  onEdit: (source: UserSource) => void;
  onDelete: (id: string) => void;
  onTest: (id: string) => void;
}

export function SourcesTable({
  sources,
  isLoading,
  onEdit,
  onDelete,
  onTest,
}: SourcesTableProps) {
  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-8">
        <p className="text-muted-foreground">Loading sources...</p>
      </div>
    );
  }

  if (sources.length === 0) {
    return (
      <div className="flex items-center justify-center py-8">
        <p className="text-muted-foreground">No sources configured yet. Create one to get started.</p>
      </div>
    );
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Name</TableHead>
          <TableHead>Type</TableHead>
          <TableHead>Status</TableHead>
          <TableHead className="text-right">Actions</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {sources.map((source) => (
          <TableRow key={source.id}>
            <TableCell className="font-medium">{source.name}</TableCell>
            <TableCell>
              <Badge variant={source.type === "rss" ? "default" : "secondary"}>
                {source.type === "rss" ? "RSS Feed" : "Web Scraper"}
              </Badge>
            </TableCell>
            <TableCell>
              {source.enabled ? (
                <div className="flex items-center gap-2 text-green-600">
                  <CheckCircle2 className="h-4 w-4" />
                  <span className="text-sm">Enabled</span>
                </div>
              ) : (
                <div className="flex items-center gap-2 text-gray-500">
                  <XCircle className="h-4 w-4" />
                  <span className="text-sm">Disabled</span>
                </div>
              )}
            </TableCell>
            <TableCell className="text-right">
              <div className="flex items-center justify-end gap-2">
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => onTest(source.id)}
                  title="Test source"
                >
                  Test
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => onEdit(source)}
                  title="Edit source"
                >
                  <Edit2 className="h-4 w-4" />
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => onDelete(source.id)}
                  title="Delete source"
                  className="text-destructive hover:text-destructive"
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
