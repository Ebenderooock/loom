import { Film, Plus } from "lucide-react";
import { titleImage } from "./title-metadata";
import { StatusBadge } from "@/components/movies/status-badge";
import { cn } from "@/lib/utils";

// Poster presentation follows the library MovieCard; actions remain separate
// buttons so opening details never accidentally adds or changes membership.
export function TitlePosterCard({
  title,
  poster,
  year,
  available,
  inLibrary = false,
  onOpen,
  onAdd,
  label = `View details for ${title}`,
  disabled = false,
}: {
  title: string;
  poster?: string;
  year?: number;
  available?: boolean;
  inLibrary?: boolean;
  onOpen: () => void;
  onAdd?: () => void;
  label?: string;
  disabled?: boolean;
}) {
  const status = available ? "available" : "missing";
  return (
    <div className="group relative min-w-0">
      <button
        type="button"
        disabled={disabled}
        aria-label={label}
        onClick={onOpen}
        className="relative block w-full overflow-hidden rounded-lg border border-transparent text-left shadow-lg transition-shadow hover:border-accent/20 hover:shadow-xl focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
      >
        <div
          className={cn(
            "absolute inset-x-0 top-0 z-10 h-[3px]",
            available ? "bg-emerald-500" : "bg-red-400",
          )}
        />
        <div className="aspect-[2/3] bg-muted">
          {poster ? (
            <img
              src={titleImage(poster, "w342")}
              alt=""
              loading="lazy"
              className="h-full w-full object-cover"
            />
          ) : (
            <div className="flex h-full items-center justify-center">
              <Film className="h-12 w-12 text-muted-foreground/30" />
            </div>
          )}
        </div>
        <div className="absolute inset-x-0 bottom-0 min-w-0 bg-gradient-to-t from-black/95 via-black/70 to-transparent p-3 pt-10">
          {available === undefined && inLibrary ? (
            <span className="rounded border border-white/20 bg-black/50 px-2 py-0.5 text-xs text-white">
              In library
            </span>
          ) : (
            <StatusBadge status={status} />
          )}
          <h3
            className="mt-1 truncate text-sm font-semibold text-white"
            title={title}
          >
            {title}
          </h3>
          <p className="text-xs text-gray-400">{year || "Date unknown"}</p>
        </div>
      </button>
      {onAdd && !inLibrary && (
        <button
          type="button"
          disabled={disabled}
          aria-label={`Add ${title}`}
          onClick={onAdd}
          className="absolute right-2 top-2 flex h-11 w-11 items-center justify-center rounded-full border border-white/20 bg-black/70 text-white shadow hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
        >
          <Plus className="h-5 w-5" />
        </button>
      )}
    </div>
  );
}
