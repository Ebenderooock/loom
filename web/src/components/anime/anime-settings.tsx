import { useEffect, useState, useCallback } from "react";
import { apiFetch } from "@/lib/fetch";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Loader2, Plus, GripVertical, X, Save } from "lucide-react";
import { toast } from "sonner";

// ─── Types ────────────────────────────────────────────────────────────

export interface AnimePreferences {
  seriesId: string;
  numberingScheme: "absolute" | "season" | "anidb";
  preferredGroups: string[];
  dualAudioRequired: boolean;
  releaseGroupScoring?: Record<string, number>;
}

export interface EpisodeMapping {
  absoluteNumber: number;
  seasonNumber: number;
  episodeNumber: number;
  anidbId?: string;
  tvdbId?: string;
}

export interface ReleaseGroup {
  name: string;
  preferred: boolean;
  score: number;
}

// ─── API helpers ──────────────────────────────────────────────────────

async function fetchPrefs(seriesId: string): Promise<AnimePreferences> {
  const res = await apiFetch(`/api/v1/anime/preferences/${seriesId}`);
  if (!res.ok) throw new Error("Failed to load anime preferences");
  return res.json();
}

async function savePrefs(prefs: AnimePreferences): Promise<void> {
  const res = await apiFetch(`/api/v1/anime/preferences/${prefs.seriesId}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(prefs),
  });
  if (!res.ok) throw new Error("Failed to save anime preferences");
}

async function fetchMappings(
  seriesId: string
): Promise<{ seriesId: string; mappings: EpisodeMapping[] }> {
  const res = await apiFetch(`/api/v1/anime/mappings/${seriesId}`);
  if (!res.ok) throw new Error("Failed to load episode mappings");
  return res.json();
}

async function fetchKnownGroups(): Promise<ReleaseGroup[]> {
  const res = await apiFetch("/api/v1/anime/groups");
  if (!res.ok) return [];
  const data = await res.json();
  return data.groups ?? [];
}

// ─── Component ────────────────────────────────────────────────────────

interface AnimeSettingsProps {
  seriesId: string;
}

export function AnimeSettings({ seriesId }: AnimeSettingsProps) {
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [prefs, setPrefs] = useState<AnimePreferences>({
    seriesId,
    numberingScheme: "absolute",
    preferredGroups: [],
    dualAudioRequired: false,
  });
  const [mappings, setMappings] = useState<EpisodeMapping[]>([]);
  const [knownGroups, setKnownGroups] = useState<ReleaseGroup[]>([]);
  const [newGroup, setNewGroup] = useState("");

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const [p, m, g] = await Promise.all([
          fetchPrefs(seriesId),
          fetchMappings(seriesId),
          fetchKnownGroups(),
        ]);
        if (cancelled) return;
        setPrefs(p);
        setMappings(m.mappings ?? []);
        setKnownGroups(g);
      } catch {
        toast.error("Failed to load anime settings");
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [seriesId]);

  const handleSave = useCallback(async () => {
    setSaving(true);
    try {
      await savePrefs(prefs);
      toast.success("Anime preferences saved");
    } catch {
      toast.error("Failed to save preferences");
    } finally {
      setSaving(false);
    }
  }, [prefs]);

  const addGroup = useCallback(() => {
    const name = newGroup.trim();
    if (!name || prefs.preferredGroups.includes(name)) return;
    setPrefs((p) => ({
      ...p,
      preferredGroups: [...p.preferredGroups, name],
    }));
    setNewGroup("");
  }, [newGroup, prefs.preferredGroups]);

  const removeGroup = useCallback((name: string) => {
    setPrefs((p) => ({
      ...p,
      preferredGroups: p.preferredGroups.filter((g) => g !== name),
    }));
  }, []);

  const moveGroup = useCallback((idx: number, dir: -1 | 1) => {
    setPrefs((p) => {
      const groups = [...p.preferredGroups];
      const target = idx + dir;
      if (target < 0 || target >= groups.length) return p;
      [groups[idx], groups[target]] = [groups[target]!, groups[idx]!];
      return { ...p, preferredGroups: groups };
    });
  }, []);

  if (loading) {
    return (
      <div className="flex items-center justify-center py-8">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Numbering Scheme */}
      <div className="space-y-2">
        <Label>Numbering Scheme</Label>
        <Select
          value={prefs.numberingScheme}
          onValueChange={(v) =>
            setPrefs((p) => ({
              ...p,
              numberingScheme: v as AnimePreferences["numberingScheme"],
            }))
          }
        >
          <SelectTrigger className="w-48">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="absolute">Absolute</SelectItem>
            <SelectItem value="season">Season</SelectItem>
            <SelectItem value="anidb">AniDB</SelectItem>
          </SelectContent>
        </Select>
        <p className="text-xs text-muted-foreground">
          How episode numbers are matched against releases
        </p>
      </div>

      {/* Dual Audio */}
      <div className="flex items-center gap-3">
        <label className="relative inline-flex cursor-pointer items-center">
          <input
            type="checkbox"
            className="peer sr-only"
            checked={prefs.dualAudioRequired}
            onChange={(e) =>
              setPrefs((p) => ({ ...p, dualAudioRequired: e.target.checked }))
            }
          />
          <div className="peer h-5 w-9 rounded-full bg-muted after:absolute after:left-[2px] after:top-[2px] after:h-4 after:w-4 after:rounded-full after:bg-white after:transition-all peer-checked:bg-primary peer-checked:after:translate-x-full" />
        </label>
        <div>
          <Label>Require Dual Audio</Label>
          <p className="text-xs text-muted-foreground">
            Only accept releases with dual audio tracks
          </p>
        </div>
      </div>

      {/* Preferred Release Groups */}
      <div className="space-y-3">
        <Label>Preferred Release Groups</Label>
        <p className="text-xs text-muted-foreground">
          Drag to reorder priority. Higher = preferred.
        </p>

        {prefs.preferredGroups.length === 0 && (
          <p className="text-sm text-muted-foreground italic">
            No preferred groups configured — all groups treated equally
          </p>
        )}

        <div className="space-y-1">
          {prefs.preferredGroups.map((g, i) => (
            <div
              key={g}
              className="flex items-center gap-2 rounded-md border px-3 py-1.5 text-sm"
            >
              <div className="flex flex-col">
                <button
                  className="text-muted-foreground hover:text-foreground disabled:opacity-30"
                  disabled={i === 0}
                  onClick={() => moveGroup(i, -1)}
                  aria-label="Move up"
                >
                  ▲
                </button>
                <button
                  className="text-muted-foreground hover:text-foreground disabled:opacity-30"
                  disabled={i === prefs.preferredGroups.length - 1}
                  onClick={() => moveGroup(i, 1)}
                  aria-label="Move down"
                >
                  ▼
                </button>
              </div>
              <GripVertical className="h-4 w-4 text-muted-foreground" />
              <span className="flex-1 font-medium">{g}</span>
              {knownGroups.find((kg) => kg.name === g) && (
                <Badge variant="secondary" className="text-xs">
                  Score:{" "}
                  {knownGroups.find((kg) => kg.name === g)?.score ?? "?"}
                </Badge>
              )}
              <button
                className="text-muted-foreground hover:text-destructive"
                onClick={() => removeGroup(g)}
                aria-label={`Remove ${g}`}
              >
                <X className="h-4 w-4" />
              </button>
            </div>
          ))}
        </div>

        <div className="flex gap-2">
          <input
            className="flex h-9 w-full rounded-md border bg-transparent px-3 py-1 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            placeholder="Add group name..."
            value={newGroup}
            onChange={(e) => setNewGroup(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && addGroup()}
          />
          <Button size="sm" variant="outline" onClick={addGroup}>
            <Plus className="mr-1 h-4 w-4" /> Add
          </Button>
        </div>

        {knownGroups.length > 0 && (
          <details className="text-sm">
            <summary className="cursor-pointer text-muted-foreground hover:text-foreground">
              Known groups ({knownGroups.length})
            </summary>
            <div className="mt-2 flex flex-wrap gap-1">
              {knownGroups.map((g) => (
                <Badge
                  key={g.name}
                  variant="outline"
                  className="cursor-pointer hover:bg-accent"
                  onClick={() => {
                    if (!prefs.preferredGroups.includes(g.name)) {
                      setPrefs((p) => ({
                        ...p,
                        preferredGroups: [...p.preferredGroups, g.name],
                      }));
                    }
                  }}
                >
                  {g.name} ({g.score})
                </Badge>
              ))}
            </div>
          </details>
        )}
      </div>

      {/* Episode Mappings Viewer */}
      {mappings.length > 0 && (
        <div className="space-y-2">
          <Label>Episode Mappings</Label>
          <Card className="max-h-60 overflow-auto">
            <table className="w-full text-sm">
              <thead className="sticky top-0 bg-card">
                <tr className="border-b text-left text-muted-foreground">
                  <th className="px-3 py-2">Absolute</th>
                  <th className="px-3 py-2">Season</th>
                  <th className="px-3 py-2">Episode</th>
                </tr>
              </thead>
              <tbody>
                {mappings.map((m) => (
                  <tr
                    key={m.absoluteNumber}
                    className="border-b last:border-0"
                  >
                    <td className="px-3 py-1.5 font-mono">
                      {m.absoluteNumber}
                    </td>
                    <td className="px-3 py-1.5">S{String(m.seasonNumber).padStart(2, "0")}</td>
                    <td className="px-3 py-1.5">E{String(m.episodeNumber).padStart(2, "0")}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </Card>
        </div>
      )}

      {/* Save */}
      <Button onClick={handleSave} disabled={saving}>
        {saving ? (
          <Loader2 className="mr-2 h-4 w-4 animate-spin" />
        ) : (
          <Save className="mr-2 h-4 w-4" />
        )}
        Save Anime Settings
      </Button>
    </div>
  );
}
