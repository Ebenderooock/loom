import * as React from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { cn } from "@/lib/utils";

const CATEGORIES = [
  { id: "general", label: "General" },
  { id: "profiles", label: "Profiles" },
  { id: "indexers", label: "Indexers" },
  { id: "download-clients", label: "Download Clients" },
  { id: "notifications", label: "Notifications" },
  { id: "connect", label: "Connect" },
  { id: "ui", label: "UI" },
] as const;

type Category = (typeof CATEGORIES)[number]["id"];

export function SettingsPage() {
  const [active, setActive] = React.useState<Category>("general");
  const activeLabel =
    CATEGORIES.find((c) => c.id === active)?.label ?? "General";

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Settings</h1>
        <p className="text-sm text-muted-foreground">
          Configure your Loom instance.
        </p>
      </div>
      <div className="grid gap-6 md:grid-cols-[14rem_1fr]">
        <nav aria-label="Settings sections">
          <ul className="flex flex-col gap-1">
            {CATEGORIES.map((c) => (
              <li key={c.id}>
                <button
                  type="button"
                  onClick={() => setActive(c.id)}
                  aria-current={active === c.id ? "page" : undefined}
                  className={cn(
                    "w-full rounded-md px-3 py-2 text-left text-sm transition-colors",
                    active === c.id
                      ? "bg-accent text-accent-foreground"
                      : "text-muted-foreground hover:bg-accent/50 hover:text-foreground",
                  )}
                >
                  {c.label}
                </button>
              </li>
            ))}
          </ul>
        </nav>
        <Card>
          <CardHeader>
            <CardTitle>{activeLabel}</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-muted-foreground">
              Settings for <strong>{activeLabel}</strong> will live here.
            </p>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
