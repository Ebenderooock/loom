import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { Layers, Loader2, X } from "lucide-react";
import {
  discoveryKeys,
  useDismissRecommendation,
  useRecommendations,
  type DiscoveryMediaType,
  type Recommendation,
} from "@/lib/discovery-api";
import { collectionKeys } from "@/lib/collections-api";
import { useAuth } from "@/hooks/use-auth";
import { TitleDialog } from "@/components/titles/title-experience";
import { TitlePosterCard } from "@/components/titles/title-poster-card";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { CardGridSkeleton } from "@/components/ui/skeletons";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";

export function RecommendationPanel() {
  const [mediaType, setMediaType] = useState<DiscoveryMediaType>("movie");
  const [includeUpcoming, setIncludeUpcoming] = useState(false);
  const [selected, setSelected] = useState<Recommendation | null>(null);
  const query = useRecommendations(mediaType, includeUpcoming);
  const qc = useQueryClient();
  const { user } = useAuth();
  return (
    <section className="space-y-5" aria-label="Grow your library">
      <div>
        <h2 className="text-xl font-semibold tracking-tight">
          What to add next
        </h2>
        <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
          New additions connected to your collection, cast, creators, and
          genres. Sourced from TMDB; no Import Lists needed.
        </p>
      </div>
      <div className="flex flex-wrap items-center justify-between gap-4">
        <Tabs
          value={mediaType}
          onValueChange={(v) => {
            if (v === "movie" || v === "series") setMediaType(v);
          }}
        >
          <TabsList aria-label="Recommendation media type">
            <TabsTrigger value="movie">Movies</TabsTrigger>
            <TabsTrigger value="series">TV shows</TabsTrigger>
          </TabsList>
        </Tabs>
        <label
          htmlFor="discovery-upcoming"
          className="flex items-center gap-2 text-sm"
        >
          <Checkbox
            id="discovery-upcoming"
            checked={includeUpcoming}
            onCheckedChange={(checked) => setIncludeUpcoming(checked === true)}
          />
          Include upcoming titles
        </label>
      </div>
      {query.isLoading ? (
        <CardGridSkeleton count={6} />
      ) : query.error ? (
        <div className="space-y-3">
          <p role="alert" className="text-sm text-destructive">
            {query.error.message}
          </p>
          <Button variant="outline" onClick={() => void query.refetch()}>
            Retry
          </Button>
        </div>
      ) : (
        query.data && (
          <>
            {query.data.warnings.map((warning) => (
              <p
                role="status"
                key={warning}
                className="text-sm text-muted-foreground"
              >
                {warning}
              </p>
            ))}
            {query.data.stale && (
              <p role="status" className="text-sm text-muted-foreground">
                Showing cached suggestions.
              </p>
            )}
            {query.data.items.length === 0 ? (
              <p className="rounded-lg border border-dashed p-8 text-center text-muted-foreground">
                No new suggestions yet. Add movies or shows with TMDB IDs to
                build your library&apos;s connections.
              </p>
            ) : (
              <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6">
                {query.data.items.map((item) => (
                  <RecommendationCard
                    key={`${item.media_type}:${item.tmdb_id}`}
                    item={item}
                    onOpen={() => setSelected(item)}
                  />
                ))}
              </div>
            )}
          </>
        )
      )}
      {selected && (
        <TitleDialog
          open
          defaults={{ search_on_add: false }}
          identity={{
            media: selected.media_type,
            id: String(selected.tmdb_id),
            title: selected.title,
            poster: selected.poster_path,
            year: selected.year,
          }}
          onOpenChange={(open) => {
            if (!open) setSelected(null);
          }}
          onAdded={() => {
            void qc.invalidateQueries({ queryKey: discoveryKeys.all });
            void qc.invalidateQueries({ queryKey: collectionKeys.all });
          }}
          requestAction={
            user?.role === "admin" ? undefined : (
              <p className="text-sm text-muted-foreground">
                Ask an administrator to add this title.
              </p>
            )
          }
        />
      )}
      <p className="text-xs text-muted-foreground">
        This product uses the TMDB API but is not endorsed or certified by TMDB.
      </p>
    </section>
  );
}

function RecommendationCard({
  item,
  onOpen,
}: {
  item: Recommendation;
  onOpen: () => void;
}) {
  const dismiss = useDismissRecommendation();
  const { user } = useAuth();
  return (
    <article className="flex min-w-0 flex-col gap-3">
      <div className="relative">
        <TitlePosterCard
          title={item.title}
          poster={item.poster_path}
          year={item.year}
          onOpen={onOpen}
          label={`View ${item.title}`}
        />
        <Button
          variant="secondary"
          size="icon"
          className="absolute right-2 top-2 h-11 w-11 shadow"
          aria-label={`Dismiss ${item.title}`}
          disabled={dismiss.isPending}
          onClick={() =>
            dismiss.mutate({
              tmdb_id: item.tmdb_id,
              media_type: item.media_type,
            })
          }
        >
          {dismiss.isPending ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <X className="h-4 w-4" />
          )}
        </Button>
      </div>
      <div className="flex flex-1 flex-col gap-3 p-3">
        {item.reasons.length > 0 && (
          <p className="text-xs leading-relaxed text-muted-foreground">
            {item.reasons.join(" · ")}
          </p>
        )}
        <div className="mt-auto space-y-2">
          <Button size="sm" className="w-full" onClick={onOpen}>
            {user?.role === "admin" ? "Add to library" : "View details"}
          </Button>
          {item.collection_id && (
            <Button variant="outline" size="sm" className="w-full" asChild>
              <Link to="/collections" search={{ tmdb: item.collection_id }}>
                <Layers className="mr-1 h-4 w-4" />
                View collection
              </Link>
            </Button>
          )}
          {dismiss.error && (
            <p role="alert" className="text-xs text-destructive">
              {dismiss.error.message}
            </p>
          )}
        </div>
      </div>
    </article>
  );
}
