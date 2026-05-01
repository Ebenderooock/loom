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
  Settings,
} from "lucide-react";

interface PaletteItem {
  label: string;
  to: "/" | "/library" | "/activity" | "/calendar" | "/settings";
  Icon: typeof LayoutDashboard;
}

const items: PaletteItem[] = [
  { label: "Dashboard", to: "/", Icon: LayoutDashboard },
  { label: "Library", to: "/library", Icon: Library },
  { label: "Activity", to: "/activity", Icon: ListTodo },
  { label: "Calendar", to: "/calendar", Icon: Calendar },
  { label: "Settings", to: "/settings", Icon: Settings },
];

interface CommandPaletteProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function CommandPalette({ open, onOpenChange }: CommandPaletteProps) {
  const navigate = useNavigate();

  const go = React.useCallback(
    (to: PaletteItem["to"]) => {
      onOpenChange(false);
      void navigate({ to });
    },
    [navigate, onOpenChange],
  );

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
