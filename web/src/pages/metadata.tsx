import * as React from "react";
import { Plus, RefreshCw, Play, SearchX } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty-state";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  useMetadataSearch,
  useMetadataImport,
  useMetadataStats,
  useProviderStatus,
  useProviderTest,
  type MovieMetadata,
  type SeriesMetadata,
  type TestResult,
  ApiError,
} from "@/lib/metadata-api";

type MediaType = "movie" | "series";
type SearchResult = MovieMetadata | SeriesMetadata;

// --- Components ---

function SearchForm({
  onSearch,
  isLoading,
}: {
  onSearch: (query: string, type: MediaType, year?: number) => void;
  isLoading: boolean;
}) {
  const [query, setQuery] = React.useState("");
  const [type, setType] = React.useState<MediaType>("movie");
  const [year, setYear] = React.useState<string>("");

  function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!query.trim()) {
      toast.error("Please enter a search query");
      return;
    }
    onSearch(query, type, year ? parseInt(year, 10) : undefined);
  }

  return (
    <form
      onSubmit={handleSubmit}
      className="space-y-4 rounded-md border border-border p-4"
    >
      <div className="grid gap-4 md:grid-cols-4">
        <div className="md:col-span-2">
          <label
            htmlFor="metadata-query"
            className="mb-2 block text-sm font-medium"
          >
            Query
          </label>
          <Input
            id="metadata-query"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="e.g., The Matrix, Breaking Bad"
            disabled={isLoading}
          />
        </div>
        <div>
          <label
            htmlFor="metadata-type"
            className="mb-2 block text-sm font-medium"
          >
            Type
          </label>
          <select
            id="metadata-type"
            value={type}
            onChange={(e) => setType(e.target.value as MediaType)}
            disabled={isLoading}
            className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
          >
            <option value="movie">Movie</option>
            <option value="series">Series</option>
          </select>
        </div>
        <div>
          <label
            htmlFor="metadata-year"
            className="mb-2 block text-sm font-medium"
          >
            Year (optional)
          </label>
          <Input
            id="metadata-year"
            type="number"
            value={year}
            onChange={(e) => setYear(e.target.value)}
            placeholder="e.g., 1999"
            disabled={isLoading}
          />
        </div>
      </div>
      <div className="flex justify-end">
        <Button type="submit" disabled={isLoading} className="gap-2">
          <Plus className="h-4 w-4" />
          {isLoading ? "Searching..." : "Search"}
        </Button>
      </div>
    </form>
  );
}

function ResultsGrid({
  results,
  isLoading,
  onImport,
  isImporting,
}: {
  results: SearchResult[];
  isLoading: boolean;
  onImport: (result: SearchResult, type: MediaType) => void;
  isImporting: boolean;
}) {
  if (isLoading) {
    return (
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
        {Array.from({ length: 6 }).map((_, i) => (
          <div
            key={i}
            className="space-y-2 rounded-md border border-border p-4"
          >
            <Skeleton className="h-48 w-full" />
            <Skeleton className="h-4 w-full" />
            <Skeleton className="h-4 w-3/4" />
          </div>
        ))}
      </div>
    );
  }

  if (!results || results.length === 0) {
    return (
      <EmptyState
        icon={<SearchX />}
        title="No results"
        description="Try a different search term or check your spelling."
      />
    );
  }

  return (
    <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
      {results.map((result, idx) => {
        const isMovie = "runtime" in result;
        const title = result.title;
        const year =
          (result as { year?: number }).year ||
          (result as { first_air_date?: string }).first_air_date?.split("-")[0];
        const poster = result.poster_path;
        const rating = result.rating;
        const overview = result.overview;

        return (
          <div
            key={
              result.tmdb_id ??
              result.imdb_id ??
              result.tvdb_id ??
              `result-${idx}`
            }
            className="overflow-hidden rounded-md border border-border transition-shadow hover:shadow-md"
          >
            {poster && (
              <img
                src={poster}
                alt={title}
                className="h-48 w-full object-cover"
              />
            )}
            {!poster && (
              <div className="flex h-48 w-full items-center justify-center bg-muted text-sm text-muted-foreground">
                No poster
              </div>
            )}
            <div className="space-y-2 p-3">
              <div>
                <h3 className="line-clamp-2 font-medium">{title}</h3>
                <div className="text-xs text-muted-foreground">
                  {isMovie ? "Movie" : "Series"} {year && `• ${year}`}
                </div>
              </div>
              {rating && (
                <div className="text-sm">⭐ {rating.toFixed(1)}/10</div>
              )}
              {overview && (
                <p className="line-clamp-2 text-xs text-muted-foreground">
                  {overview}
                </p>
              )}
              <Button
                size="sm"
                className="mt-2 w-full"
                onClick={() => onImport(result, isMovie ? "movie" : "series")}
                disabled={isImporting}
              >
                Import
              </Button>
            </div>
          </div>
        );
      })}
    </div>
  );
}

function CacheStatsTable() {
  const {
    data: stats,
    isLoading,
    isError,
    error,
    refetch,
  } = useMetadataStats();

  if (isLoading) {
    return (
      <div className="space-y-2">
        <Skeleton className="h-12 w-full" />
        <Skeleton className="h-12 w-full" />
        <Skeleton className="h-12 w-full" />
      </div>
    );
  }

  if (isError) {
    return (
      <div className="rounded-md border border-red-500/40 bg-red-500/10 p-3 text-sm text-red-700">
        Failed to load cache stats:{" "}
        {error instanceof Error ? error.message : "Unknown error"}
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex justify-end">
        <Button
          size="sm"
          variant="outline"
          onClick={() => refetch()}
          className="gap-2"
        >
          <RefreshCw className="h-4 w-4" />
          Refresh
        </Button>
      </div>
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        <div className="rounded-md border border-border p-4">
          <div className="text-sm text-muted-foreground">Hit Rate</div>
          <div className="text-2xl font-semibold">
            {stats?.hit_rate?.toFixed(1) ?? 0}%
          </div>
        </div>
        <div className="rounded-md border border-border p-4">
          <div className="text-sm text-muted-foreground">Miss Rate</div>
          <div className="text-2xl font-semibold">
            {stats?.miss_rate?.toFixed(1) ?? 0}%
          </div>
        </div>
        <div className="rounded-md border border-border p-4">
          <div className="text-sm text-muted-foreground">Cache Size</div>
          <div className="text-2xl font-semibold">
            {stats?.cache_size ?? 0} KB
          </div>
        </div>
        <div className="rounded-md border border-border p-4">
          <div className="text-sm text-muted-foreground">Entries</div>
          <div className="text-2xl font-semibold">{stats?.entries ?? 0}</div>
        </div>
      </div>
    </div>
  );
}

function ProviderStatusCard({
  provider,
}: {
  provider: "tmdb" | "tvdb" | "musicbrainz";
}) {
  const { data: status, isLoading } = useProviderStatus(provider);
  const test = useProviderTest(provider);
  const [testResult, setTestResult] = React.useState<TestResult | null>(null);

  async function handleTest() {
    try {
      const result = await test.mutateAsync();
      setTestResult(result);
      if (result.ok) {
        toast.success(`${provider} test passed (${result.latency_ms}ms)`);
      } else {
        toast.error(`${provider} test failed: ${result.error}`);
      }
    } catch (err) {
      const message = err instanceof ApiError ? err.message : String(err);
      toast.error(`Test failed: ${message}`);
    }
  }

  const statusBadge =
    !status || status.status === "unconfigured" ? (
      <span className="inline-flex items-center gap-1 rounded bg-yellow-100 px-2 py-1 text-xs font-medium text-yellow-700">
        ⚠ Unconfigured
      </span>
    ) : status.status === "error" ? (
      <span className="inline-flex items-center gap-1 rounded bg-red-100 px-2 py-1 text-xs font-medium text-red-700">
        ✗ Error
      </span>
    ) : (
      <span className="inline-flex items-center gap-1 rounded bg-green-100 px-2 py-1 text-xs font-medium text-green-700">
        ✓ OK
      </span>
    );

  return (
    <div className="rounded-md border border-border p-4">
      <div className="mb-3 flex items-start justify-between">
        <div>
          <h3 className="font-medium capitalize">{provider}</h3>
          <div className="mt-1">{statusBadge}</div>
        </div>
      </div>
      <div className="mb-3 space-y-1 text-xs text-muted-foreground">
        <div>
          API Key:{" "}
          {status?.configured_api_key ? "✓ Configured" : "✗ Not configured"}
        </div>
        {status?.last_test_time && (
          <div>
            Last test: {new Date(status.last_test_time).toLocaleString()}
          </div>
        )}
        {status?.last_test_latency_ms && (
          <div>Latency: {status.last_test_latency_ms}ms</div>
        )}
      </div>
      <Button
        size="sm"
        onClick={handleTest}
        disabled={test.isPending || isLoading}
        className="w-full gap-2"
      >
        <Play className="h-3 w-3" />
        {test.isPending ? "Testing..." : "Test"}
      </Button>
      {testResult && (
        <div className="mt-2 rounded bg-muted p-2 text-xs text-muted-foreground">
          {testResult.ok ? (
            <div>✓ Test passed ({testResult.latency_ms}ms)</div>
          ) : (
            <div>✗ {testResult.error}</div>
          )}
        </div>
      )}
    </div>
  );
}

// --- Main Page ---

export function MetadataPage() {
  const search = useMetadataSearch();
  const importMutation = useMetadataImport();
  const [results, setResults] = React.useState<SearchResult[]>([]);
  const [importDialog, setImportDialog] = React.useState<{
    result: SearchResult;
    type: MediaType;
  } | null>(null);

  async function handleSearch(query: string, type: MediaType, year?: number) {
    try {
      const results = await search.mutateAsync({ query, type, year });
      setResults((results as SearchResult[]) || []);
      toast.success(
        `Found ${(results as SearchResult[])?.length ?? 0} results`,
      );
    } catch (err) {
      const message = err instanceof ApiError ? err.message : String(err);
      toast.error(`Search failed: ${message}`);
    }
  }

  async function handleImport(result: SearchResult, type: MediaType) {
    try {
      await importMutation.mutateAsync({ type, metadata: result });
      toast.success(`Imported: ${result.title}`);
      setImportDialog(null);
    } catch (err) {
      const message = err instanceof ApiError ? err.message : String(err);
      toast.error(`Import failed: ${message}`);
    }
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Metadata</h1>
        <p className="text-sm text-muted-foreground">
          Search for movies and TV series metadata across all providers.
        </p>
      </div>

      <Tabs defaultValue="search" className="w-full">
        <TabsList className="grid w-full grid-cols-3">
          <TabsTrigger value="search">Search</TabsTrigger>
          <TabsTrigger value="cache">Cache Stats</TabsTrigger>
          <TabsTrigger value="providers">Provider Status</TabsTrigger>
        </TabsList>

        <TabsContent value="search" className="mt-6 space-y-6">
          <SearchForm onSearch={handleSearch} isLoading={search.isPending} />
          <ResultsGrid
            results={results}
            isLoading={search.isPending}
            onImport={(result, type) => setImportDialog({ result, type })}
            isImporting={importMutation.isPending}
          />
        </TabsContent>

        <TabsContent value="cache" className="mt-6 space-y-6">
          <CacheStatsTable />
        </TabsContent>

        <TabsContent value="providers" className="mt-6 space-y-6">
          <div className="grid gap-4 md:grid-cols-3">
            <ProviderStatusCard provider="tmdb" />
            <ProviderStatusCard provider="tvdb" />
            <ProviderStatusCard provider="musicbrainz" />
          </div>
        </TabsContent>
      </Tabs>

      <Dialog
        open={!!importDialog}
        onOpenChange={(open) => !open && setImportDialog(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Confirm Import</DialogTitle>
            <DialogDescription>
              Are you sure you want to import "{importDialog?.result.title}"? It
              will be cached for 7 days.
            </DialogDescription>
          </DialogHeader>
          <div className="max-h-64 space-y-3 overflow-y-auto">
            <div>
              <div className="text-sm font-medium">Title</div>
              <div className="text-sm text-muted-foreground">
                {importDialog?.result.title}
              </div>
            </div>
            {importDialog?.result.overview && (
              <div>
                <div className="text-sm font-medium">Overview</div>
                <div className="line-clamp-3 text-sm text-muted-foreground">
                  {importDialog.result.overview}
                </div>
              </div>
            )}
            {importDialog?.result.rating && (
              <div>
                <div className="text-sm font-medium">Rating</div>
                <div className="text-sm text-muted-foreground">
                  ⭐ {importDialog.result.rating.toFixed(1)}/10
                </div>
              </div>
            )}
          </div>
          <div className="flex justify-end gap-2">
            <Button
              variant="outline"
              onClick={() => setImportDialog(null)}
              disabled={importMutation.isPending}
            >
              Cancel
            </Button>
            <Button
              onClick={() => {
                if (importDialog) {
                  handleImport(importDialog.result, importDialog.type);
                }
              }}
              disabled={importMutation.isPending}
            >
              {importMutation.isPending ? "Importing..." : "Import"}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
