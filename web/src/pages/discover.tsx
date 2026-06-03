import * as React from "react";
import {
  useDiscover,
  useAddDiscoverItem,
  type DiscoverItem,
  type MediaType,
} from "@/lib/import-lists-api";
import { usePageHeader } from "@/hooks/use-page-header";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Check, Loader2, Plus, Compass } from "lucide-react";

export function DiscoverPage() {
  const { setHeader } = usePageHeader();
  React.useEffect(() => setHeader({ title: "Discover" }), [setHeader]);

  return (
    <div className="space-y-6 p-6">
      <Tabs defaultValue="movie">
        <TabsList>
          <TabsTrigger value="movie">Movies</TabsTrigger>
          <TabsTrigger value="series">TV Shows</TabsTrigger>
        </TabsList>

        <TabsContent value="movie" className="mt-4">
          <DiscoverGrid mediaType="movie" />
        </TabsContent>
        <TabsContent value="series" className="mt-4">
          <DiscoverGrid mediaType="series" />
        </TabsContent>
      </Tabs>
    </div>
  );
}

function DiscoverGrid({ mediaType }: { mediaType: MediaType }) {
  const { data: items, isLoading, isError, error } = useDiscover(mediaType);

  if (isLoading) {
    return (
      <p className="text-sm text-muted-foreground">Loading…</p>
    );
  }
  if (isError) {
    return (
      <p className="text-sm text-destructive">
        {error?.message ?? "Failed to load discover feed."}
      </p>
    );
  }
  if (!items || items.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center gap-3 py-16 text-center">
        <Compass className="h-10 w-10 text-muted-foreground" />
        <p className="text-sm font-medium">Nothing to discover yet</p>
        <p className="max-w-md text-xs text-muted-foreground">
          Set an import list to <span className="font-medium">Discover</span>{" "}
          mode in Import Lists, then sync it. Its{" "}
          {mediaType === "series" ? "shows" : "movies"} will appear here for you
          to add.
        </p>
      </div>
    );
  }

  return (
    <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
      {items.map((item) => (
        <DiscoverCard key={item.id} item={item} />
      ))}
    </div>
  );
}

function DiscoverCard({ item }: { item: DiscoverItem }) {
  const addMut = useAddDiscoverItem();
  const [added, setAdded] = React.useState(false);

  const inLibrary = item.in_library || added;

  const handleAdd = () => {
    addMut.mutate(item.id, { onSuccess: () => setAdded(true) });
  };

  return (
    <div className="group flex flex-col overflow-hidden rounded-lg border bg-card">
      <div className="relative aspect-[2/3] w-full bg-muted">
        {item.poster_path ? (
          <img
            src={item.poster_path}
            alt={item.title}
            loading="lazy"
            className="h-full w-full object-cover"
          />
        ) : (
          <div className="flex h-full w-full items-center justify-center p-2 text-center text-xs text-muted-foreground">
            {item.title}
          </div>
        )}
        {inLibrary && (
          <Badge className="absolute left-2 top-2" variant="secondary">
            In Library
          </Badge>
        )}
      </div>

      <div className="flex flex-1 flex-col gap-2 p-3">
        <div className="min-h-[2.5rem]">
          <p className="line-clamp-2 text-sm font-medium" title={item.title}>
            {item.title}
          </p>
          {item.year ? (
            <p className="text-xs text-muted-foreground">{item.year}</p>
          ) : null}
        </div>

        {inLibrary ? (
          <Button size="sm" variant="ghost" disabled className="w-full">
            <Check className="mr-1 h-4 w-4" />
            Added
          </Button>
        ) : (
          <Button
            size="sm"
            className="w-full"
            disabled={addMut.isPending}
            onClick={handleAdd}
          >
            {addMut.isPending ? (
              <Loader2 className="mr-1 h-4 w-4 animate-spin" />
            ) : (
              <Plus className="mr-1 h-4 w-4" />
            )}
            {addMut.isPending ? "Adding…" : "Add"}
          </Button>
        )}

        {addMut.isError && (
          <p className="text-xs text-destructive">
            {(addMut.error as Error)?.message ?? "Failed to add"}
          </p>
        )}
      </div>
    </div>
  );
}
