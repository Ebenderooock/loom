import * as React from "react";
import { Command } from "cmdk";
import { useNavigate } from "@tanstack/react-router";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Calendar,
  LayoutDashboard,
  Library,
  ListTodo,
  Search,
  Settings,
  X,
  Film,
  Tv,
} from "lucide-react";

interface PaletteItem {
  label: string;
  to: "/" | "/library" | "/movies" | "/series" | "/activity" | "/calendar" | "/settings";
  Icon: typeof LayoutDashboard;
}

const items: PaletteItem[] = [
  { label: "Dashboard", to: "/", Icon: LayoutDashboard },
  { label: "Movies", to: "/movies", Icon: Film },
  { label: "TV Shows", to: "/series", Icon: Tv },
  { label: "Library", to: "/library", Icon: Library },
  { label: "Activity", to: "/activity", Icon: ListTodo },
  { label: "Calendar", to: "/calendar", Icon: Calendar },
  { label: "Settings", to: "/settings", Icon: Settings },
];

interface CommandPaletteProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  inline?: boolean;
}

export function CommandPalette({ open, onOpenChange, inline }: CommandPaletteProps) {
  const navigate = useNavigate();
  const inputRef = React.useRef<HTMLInputElement>(null);

  const go = React.useCallback(
    (to: PaletteItem["to"]) => {
      onOpenChange(false);
      void navigate({ to });
    },
    [navigate, onOpenChange],
  );

  React.useEffect(() => {
    if (open && inline) {
      // Focus the input after a tick so the DOM has rendered
      requestAnimationFrame(() => inputRef.current?.focus());
    }
  }, [open, inline]);

  if (inline) {
    if (!open) return null;
    return (
      <Command label="Command Menu" className="flex flex-col flex-1 relative">
        {/* Search input — fills the header bar */}
        <div className="flex items-center flex-1 gap-2">
          <Search className="w-4 h-4 text-muted-foreground shrink-0" />
          <Command.Input
            ref={inputRef}
            placeholder="Search Loom — navigate, find movies, series…"
            className="flex-1 h-10 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
          />
          <kbd className="rounded border border-border bg-muted px-1.5 py-0.5 font-mono text-[10px] font-medium text-muted-foreground">
            ESC
          </kbd>
          <button
            onClick={() => onOpenChange(false)}
            className="p-1.5 rounded-md hover:bg-muted text-muted-foreground hover:text-foreground transition-colors"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Results panel — drops below the header */}
        <Command.List className="absolute top-full left-0 right-0 max-h-80 overflow-y-auto bg-background border border-border border-t-0 rounded-b-lg shadow-xl p-2">
          <Command.Empty className="py-6 text-center text-sm text-muted-foreground">
            No results found.
          </Command.Empty>
          <Command.Group heading="Navigation" className="text-xs text-muted-foreground px-2 py-1">
            {items.map(({ label, to, Icon }) => (
              <Command.Item
                key={to}
                value={label}
                onSelect={() => go(to)}
                className="flex cursor-pointer items-center gap-2 rounded-md px-3 py-2 text-sm aria-selected:bg-accent aria-selected:text-accent-foreground"
              >
                <Icon className="h-4 w-4" />
                <span>{label}</span>
              </Command.Item>
            ))}
          </Command.Group>
        </Command.List>
      </Command>
    );
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="overflow-hidden p-0">
        <DialogTitle className="sr-only">Command Palette</DialogTitle>
        <DialogDescription className="sr-only">
          Quickly navigate Loom by typing a route name.
        </DialogDescription>
        <Command label="Command Menu" className="flex flex-col">
          <Command.Input
            placeholder="Type a command or search…"
            className="h-12 border-b border-border bg-transparent px-4 text-sm outline-none placeholder:text-muted-foreground"
          />
          <Command.List className="max-h-80 overflow-y-auto p-2">
            <Command.Empty className="py-6 text-center text-sm text-muted-foreground">
              No results found.
            </Command.Empty>
            <Command.Group heading="Navigation" className="text-xs">
              {items.map(({ label, to, Icon }) => (
                <Command.Item
                  key={to}
                  value={label}
                  onSelect={() => go(to)}
                  className="flex cursor-pointer items-center gap-2 rounded-md px-3 py-2 text-sm aria-selected:bg-accent aria-selected:text-accent-foreground"
                >
                  <Icon className="h-4 w-4" />
                  <span>{label}</span>
                </Command.Item>
              ))}
            </Command.Group>
          </Command.List>
        </Command>
      </DialogContent>
    </Dialog>
  );
}

export function useCommandPalette() {
  const [open, setOpen] = React.useState(false);

  React.useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.key === "k" || e.key === "K") && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        setOpen((v) => !v);
      }
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, []);

  return { open, setOpen };
}
