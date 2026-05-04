import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { AlertCircle, Plus, Trash2, Loader2 } from "lucide-react";
import { useAuth } from "@/hooks/use-auth";

interface RootFolder {
  id: string;
  path: string;
  createdAt: string;
}

interface Movie {
  id: string;
  title: string;
  year: number;
  tmdbId: number;
  posterPath?: string;
  monitoringStatus: string;
}

export function MoviesPage() {
  const { isAuthenticated } = useAuth();
  const [rootFolders, setRootFolders] = useState<RootFolder[]>([]);
  const [movies, setMovies] = useState<Movie[]>([]);
  const [newPath, setNewPath] = useState("");
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  // Fetch root folders on mount (only when authenticated)
  useEffect(() => {
    if (isAuthenticated) {
      fetchRootFolders();
      fetchMovies();
    }
  }, [isAuthenticated]);

  const fetchRootFolders = async () => {
    try {
      setError(null);
      const response = await fetch("http://localhost:8989/api/v1/movies/root-folders", {
        credentials: "include",
      });

      if (!response.ok) throw new Error("Failed to fetch root folders");
      const data = await response.json();
      setRootFolders(data || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to fetch root folders");
    }
  };

  const fetchMovies = async () => {
    try {
      const response = await fetch("http://localhost:8989/api/v1/movies/", {
        credentials: "include",
      });

      if (!response.ok) throw new Error("Failed to fetch movies");
      const data = await response.json();
      // Handle both paginated response {data: [...]} and direct array response
      const moviesList = Array.isArray(data) ? data : (data?.data || []);
      setMovies(moviesList);
    } catch (err) {
      console.error("Failed to fetch movies:", err);
      setError("Failed to fetch movies");
    }
  };

  const addRootFolder = async () => {
    if (!newPath.trim()) {
      setError("Path is required");
      return;
    }

    setIsLoading(true);
    setError(null);
    setSuccess(null);

    try {
      const response = await fetch("http://localhost:8989/api/v1/movies/root-folders", {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ path: newPath.trim() }),
      });

      if (!response.ok) {
        const err = await response.json().catch(() => ({}));
        throw new Error(err.error || "Failed to add root folder");
      }

      setNewPath("");
      setSuccess("Root folder added successfully");
      fetchRootFolders();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to add root folder");
    } finally {
      setIsLoading(false);
    }
  };

  const deleteRootFolder = async (id: string) => {
    if (!confirm("Are you sure you want to delete this root folder?")) return;

    setIsLoading(true);
    setError(null);

    try {
      const response = await fetch(
        `http://localhost:8989/api/v1/movies/root-folders/${id}`,
        {
          method: "DELETE",
          credentials: "include",
        }
      );

      if (!response.ok) throw new Error("Failed to delete root folder");

      setSuccess("Root folder deleted successfully");
      fetchRootFolders();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to delete root folder");
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Movies</h1>
        <p className="text-sm text-muted-foreground">
          Manage your movie library and settings
        </p>
      </div>

      {/* Add Root Folder Section */}
      <Card>
        <CardHeader>
          <CardTitle>Library Folders</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {error && (
            <Alert className="border-destructive/50 bg-destructive/10">
              <AlertCircle className="h-4 w-4" />
              <AlertDescription className="text-destructive">{error}</AlertDescription>
            </Alert>
          )}

          {success && (
            <Alert className="border-green-500/50 bg-green-500/10">
              <AlertCircle className="h-4 w-4 text-green-600" />
              <AlertDescription className="text-green-600">{success}</AlertDescription>
            </Alert>
          )}

          <div className="flex gap-2">
            <Input
              placeholder="/mnt/movies or /home/user/movies"
              value={newPath}
              onChange={(e) => setNewPath(e.target.value)}
              disabled={isLoading}
              className="flex-1"
            />
            <Button
              onClick={addRootFolder}
              disabled={isLoading || !newPath.trim()}
              className="gap-2"
            >
              {isLoading ? <Loader2 className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}
              Add Folder
            </Button>
          </div>

          {rootFolders.length > 0 ? (
            <div className="space-y-2">
              {rootFolders.map((folder) => (
                <div
                  key={folder.id}
                  className="flex items-center justify-between p-3 bg-secondary rounded-lg"
                >
                  <div className="min-w-0">
                    <p className="font-medium truncate">{folder.path}</p>
                    <p className="text-sm text-muted-foreground">
                      Added {new Date(folder.createdAt).toLocaleDateString()}
                    </p>
                  </div>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => deleteRootFolder(folder.id)}
                    disabled={isLoading}
                    className="text-destructive hover:text-destructive"
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              ))}
            </div>
          ) : (
            <p className="text-sm text-muted-foreground text-center py-4">
              No library folders configured yet
            </p>
          )}
        </CardContent>
      </Card>

      {/* Movies List Section */}
      <Card>
        <CardHeader>
          <CardTitle>Your Movies ({movies.length})</CardTitle>
        </CardHeader>
        <CardContent>
          {movies.length > 0 ? (
            <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 xl:grid-cols-6">
              {movies.map((movie) => (
                <div key={movie.id} className="group cursor-pointer">
                  <div className="aspect-[2/3] bg-secondary rounded-lg overflow-hidden mb-2 flex items-center justify-center">
                    {movie.posterPath ? (
                      <img
                        src={movie.posterPath}
                        alt={movie.title}
                        className="w-full h-full object-cover group-hover:scale-105 transition-transform"
                      />
                    ) : (
                      <div className="text-center p-2">
                        <p className="text-xs font-medium text-muted-foreground">{movie.title}</p>
                      </div>
                    )}
                  </div>
                  <p className="text-sm font-medium truncate">{movie.title}</p>
                  <p className="text-xs text-muted-foreground">{movie.year}</p>
                  <p className="text-xs text-muted-foreground mt-1">
                    {movie.monitoringStatus}
                  </p>
                </div>
              ))}
            </div>
          ) : (
            <p className="text-sm text-muted-foreground text-center py-8">
              No movies found. Add library folders and scan them to discover movies.
            </p>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
