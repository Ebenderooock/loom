import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Plus } from "lucide-react";
import { useToast } from "@/hooks/use-toast";
import { useSetPageHeader } from "@/hooks/use-page-header";
import {
  useSources,
  useCreateSource,
  useUpdateSource,
  useDeleteSource,
  useTestSource,
} from "@/lib/sources-api";
import type {
  UserSource,
  UserSourceCreate,
  UserSourcePatch,
} from "@/lib/sources-api";
import { SourcesTable } from "@/components/sources/SourcesTable";
import { SourceForm } from "@/components/sources/SourceForm";
import { SourceTestPreview } from "@/components/sources/SourceTestPreview";

export function SourcesPage() {
  useSetPageHeader("Sources");
  const { toast } = useToast();
  const [formOpen, setFormOpen] = useState(false);
  const [editingSource, setEditingSource] = useState<UserSource | undefined>();
  const [previewOpen, setPreviewOpen] = useState(false);
  const [previewSource, setPreviewSource] = useState<UserSource | undefined>();

  const { data: sources, isLoading: sourcesLoading } = useSources();
  const createMutation = useCreateSource();
  const updateMutation = useUpdateSource();
  const deleteMutation = useDeleteSource();
  const testMutation = useTestSource();

  const handleOpenAdd = () => {
    setEditingSource(undefined);
    setFormOpen(true);
  };

  const handleEdit = (source: UserSource) => {
    setEditingSource(source);
    setFormOpen(true);
  };

  const handleCloseForm = () => {
    setFormOpen(false);
    setEditingSource(undefined);
  };

  const handleSave = async (
    data: UserSourceCreate | { id: string; patch: UserSourcePatch },
  ) => {
    try {
      if ("id" in data) {
        await updateMutation.mutateAsync(data);
        toast({
          title: "Success",
          description: "Source updated successfully",
        });
      } else {
        await createMutation.mutateAsync(data);
        toast({
          title: "Success",
          description: "Source created successfully",
        });
      }
      handleCloseForm();
    } catch (error) {
      toast({
        title: "Error",
        description:
          error instanceof Error ? error.message : "Failed to save source",
        variant: "destructive",
      });
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm("Are you sure you want to delete this source?")) {
      return;
    }

    try {
      await deleteMutation.mutateAsync(id);
      toast({
        title: "Success",
        description: "Source deleted successfully",
      });
    } catch (error) {
      toast({
        title: "Error",
        description:
          error instanceof Error ? error.message : "Failed to delete source",
        variant: "destructive",
      });
    }
  };

  const handleTest = async (id: string) => {
    const source = sources?.find((s) => s.id === id);
    if (!source) return;

    setPreviewSource(source);
    setPreviewOpen(true);

    try {
      const result = await testMutation.mutateAsync(id);
      if (result.success) {
        toast({
          title: "Test Successful",
          description: `Found ${result.items?.length ?? 0} items`,
        });
      } else {
        toast({
          title: "Test Failed",
          description: result.error || "Unknown error",
          variant: "destructive",
        });
      }
    } catch (error) {
      toast({
        title: "Test Error",
        description:
          error instanceof Error ? error.message : "Failed to test source",
        variant: "destructive",
      });
    }
  };

  const handleClosePreview = () => {
    setPreviewOpen(false);
    setPreviewSource(undefined);
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <Button onClick={handleOpenAdd} className="gap-2">
          <Plus className="h-4 w-4" />
          Add Source
        </Button>
      </div>

      <div className="rounded-lg border">
        <SourcesTable
          sources={sources ?? []}
          isLoading={sourcesLoading}
          onEdit={handleEdit}
          onDelete={handleDelete}
          onTest={handleTest}
        />
      </div>

      <SourceForm
        open={formOpen}
        source={editingSource}
        onClose={handleCloseForm}
        onSave={handleSave}
        isLoading={createMutation.isPending || updateMutation.isPending}
      />

      <SourceTestPreview
        open={previewOpen}
        onClose={handleClosePreview}
        isLoading={testMutation.isPending}
        result={testMutation.data}
        error={
          testMutation.error instanceof Error ? testMutation.error : undefined
        }
        sourceName={previewSource?.name}
      />
    </div>
  );
}
