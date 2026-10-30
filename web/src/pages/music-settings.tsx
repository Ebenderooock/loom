import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import * as React from "react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Trash2 } from "lucide-react";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  useAudioQualityDefinitions,
  useAudioQualityProfiles,
  useCreateAudioQualityProfile,
  useDeleteAudioQualityProfile,
  useCreateMetadataProfile,
  useDeleteMetadataProfile,
  useMetadataProfiles,
  useUpdateMetadataProfile,
  useUpdateAudioQualityProfile,
  type MetadataProfile,
  type AudioQualityProfile,
} from "@/lib/music-api";
import { useCustomFormats } from "@/lib/custom-formats-api";

interface ProfileItem {
  definition_id: string;
  allowed: boolean;
}

function parseItems(profile: AudioQualityProfile): ProfileItem[] {
  const raw = profile.items;
  if (Array.isArray(raw)) return raw as ProfileItem[];
  if (typeof raw === "string") {
    try {
      return JSON.parse(raw) as ProfileItem[];
    } catch {
      return [];
    }
  }
  return [];
}

function AudioProfileCard({
  profile,
  definitions,
  defName,
}: {
  profile: AudioQualityProfile;
  definitions: { id: string; name: string }[];
  defName: (id: string) => string;
}) {
  const items = parseItems(profile).filter((i) => i.allowed);
  const { data: customFormats = [] } = useCustomFormats();
  const update = useUpdateAudioQualityProfile();
  const remove = useDeleteAudioQualityProfile();

  const initialScores = React.useMemo(() => {
    const m: Record<string, number> = {};
    for (const fi of profile.format_items ?? []) m[fi.format_id] = fi.score;
    return m;
  }, [profile.format_items]);

  const [scores, setScores] =
    React.useState<Record<string, number>>(initialScores);
  const [minScore, setMinScore] = React.useState<number>(
    profile.min_format_score ?? 0,
  );
  const [allowed, setAllowed] = React.useState<Record<string, boolean>>(() =>
    Object.fromEntries(
      parseItems(profile).map((item) => [item.definition_id, item.allowed]),
    ),
  );
  const [cutoff, setCutoff] = React.useState(profile.cutoff ?? "");
  const [upgradeAllowed, setUpgradeAllowed] = React.useState(
    profile.upgrade_allowed,
  );

  React.useEffect(() => {
    setScores(initialScores);
    setMinScore(profile.min_format_score ?? 0);
    setAllowed(
      Object.fromEntries(
        parseItems(profile).map((item) => [item.definition_id, item.allowed]),
      ),
    );
    setCutoff(profile.cutoff ?? "");
    setUpgradeAllowed(profile.upgrade_allowed);
  }, [initialScores, profile]);

  const dirty =
    minScore !== (profile.min_format_score ?? 0) ||
    cutoff !== (profile.cutoff ?? "") ||
    upgradeAllowed !== profile.upgrade_allowed ||
    definitions.some(
      (definition) =>
        (allowed[definition.id] ?? false) !==
        (parseItems(profile).find(
          (item) => item.definition_id === definition.id,
        )?.allowed ?? false),
    ) ||
    customFormats.some(
      (cf) => (scores[cf.id] ?? 0) !== (initialScores[cf.id] ?? 0),
    );

  const save = () => {
    const format_items = customFormats
      .map((cf) => ({ format_id: cf.id, score: scores[cf.id] ?? 0 }))
      .filter((fi) => fi.score !== 0);
    update.mutate(
      {
        id: profile.id,
        req: {
          format_items,
          min_format_score: minScore,
          items: definitions.map((definition) => ({
            definition_id: definition.id,
            allowed: allowed[definition.id] ?? false,
          })),
          cutoff,
          upgrade_allowed: upgradeAllowed,
        },
      },
      {
        onSuccess: () => toast.success(`Saved scoring for ${profile.name}`),
        onError: (e) =>
          toast.error(e instanceof Error ? e.message : "Save failed"),
      },
    );
  };

  return (
    <div className="rounded-md border p-3">
      <div className="mb-2 flex items-center justify-between">
        <span className="font-medium">{profile.name}</span>
        <div className="flex items-center gap-2">
          <span className="text-xs text-muted-foreground">
            Cutoff: {profile.cutoff ? defName(profile.cutoff) : "—"}
            {profile.upgrade_allowed ? " · upgrades on" : " · upgrades off"}
          </span>
          <Button
            size="icon"
            variant="ghost"
            aria-label={`Delete ${profile.name}`}
            disabled={remove.isPending}
            onClick={() =>
              remove.mutate(profile.id, {
                onSuccess: () => toast.success(`Deleted ${profile.name}`),
                onError: (e) =>
                  toast.error(e instanceof Error ? e.message : "Delete failed"),
              })
            }
          >
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>
      </div>
      <div className="mb-3 flex flex-wrap gap-1.5">
        {items.length === 0 ? (
          <span className="text-xs text-muted-foreground">
            No qualities allowed.
          </span>
        ) : (
          items.map((i) => (
            <Badge key={i.definition_id} variant="secondary">
              {defName(i.definition_id)}
            </Badge>
          ))
        )}
      </div>

      <div className="mt-3 border-t pt-3">
        <div className="mb-2 text-xs font-medium text-muted-foreground">
          Allowed audio qualities
        </div>
        <div className="grid gap-1 sm:grid-cols-2">
          {definitions.map((definition) => (
            <label
              key={definition.id}
              className="flex items-center gap-2 text-sm"
            >
              <input
                type="checkbox"
                checked={allowed[definition.id] ?? false}
                onChange={(event) =>
                  setAllowed((current) => ({
                    ...current,
                    [definition.id]: event.target.checked,
                  }))
                }
              />
              {definition.name}
            </label>
          ))}
        </div>
        <div className="mt-3 grid gap-2 sm:grid-cols-2">
          <label className="flex items-center gap-2 text-sm">
            Cutoff
            <select
              className="h-8 rounded-md border bg-background px-2"
              value={cutoff}
              onChange={(event) => setCutoff(event.target.value)}
            >
              <option value="">None</option>
              {definitions.map((definition) => (
                <option key={definition.id} value={definition.id}>
                  {definition.name}
                </option>
              ))}
            </select>
          </label>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={upgradeAllowed}
              onChange={(event) => setUpgradeAllowed(event.target.checked)}
            />
            Allow quality upgrades
          </label>
        </div>
      </div>

      <div className="mt-3 border-t pt-3">
        <div className="mb-2 text-xs font-medium text-muted-foreground">
          Custom format scores
        </div>
        {customFormats.length === 0 ? (
          <p className="text-xs text-muted-foreground">
            No custom formats defined.
          </p>
        ) : (
          <div className="space-y-1.5">
            {customFormats.map((cf) => (
              <div
                key={cf.id}
                className="flex items-center justify-between gap-2"
              >
                <span className="text-sm">{cf.name}</span>
                <Input
                  type="number"
                  className="h-7 w-24 text-right tabular-nums"
                  value={scores[cf.id] ?? 0}
                  onChange={(e) =>
                    setScores((s) => ({
                      ...s,
                      [cf.id]: Number(e.target.value) || 0,
                    }))
                  }
                />
              </div>
            ))}
          </div>
        )}
        <div className="mt-3 flex items-center justify-between gap-2">
          <Label htmlFor={`min-${profile.id}`} className="text-sm">
            Minimum custom format score
          </Label>
          <Input
            id={`min-${profile.id}`}
            type="number"
            className="h-7 w-24 text-right tabular-nums"
            value={minScore}
            onChange={(e) => setMinScore(Number(e.target.value) || 0)}
          />
        </div>
        <div className="mt-3 flex justify-end">
          <Button
            size="sm"
            disabled={!dirty || update.isPending}
            onClick={save}
          >
            {update.isPending ? "Saving…" : "Save scoring"}
          </Button>
        </div>
      </div>
    </div>
  );
}

function splitProfileValues(value: string): string[] {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

function MetadataProfileCard({ profile }: { profile: MetadataProfile }) {
  const update = useUpdateMetadataProfile();
  const remove = useDeleteMetadataProfile();
  const [name, setName] = React.useState(profile.name);
  const [primary, setPrimary] = React.useState(
    (profile.primary_types ?? []).join(", "),
  );
  const [secondary, setSecondary] = React.useState(
    (profile.secondary_types ?? []).join(", "),
  );
  const [statuses, setStatuses] = React.useState(
    (profile.release_statuses ?? []).join(", "),
  );
  React.useEffect(() => {
    setName(profile.name);
    setPrimary((profile.primary_types ?? []).join(", "));
    setSecondary((profile.secondary_types ?? []).join(", "));
    setStatuses((profile.release_statuses ?? []).join(", "));
  }, [profile]);
  const dirty =
    name !== profile.name ||
    primary !== (profile.primary_types ?? []).join(", ") ||
    secondary !== (profile.secondary_types ?? []).join(", ") ||
    statuses !== (profile.release_statuses ?? []).join(", ");

  return (
    <div className="space-y-2 rounded-md border p-3">
      <div className="grid gap-2 sm:grid-cols-2">
        <Label className="space-y-1">
          Name
          <Input
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
        </Label>
        <Label className="space-y-1">
          Primary types
          <Input
            value={primary}
            onChange={(event) => setPrimary(event.target.value)}
          />
        </Label>
        <Label className="space-y-1">
          Secondary types
          <Input
            value={secondary}
            onChange={(event) => setSecondary(event.target.value)}
          />
        </Label>
        <Label className="space-y-1">
          Release statuses
          <Input
            value={statuses}
            onChange={(event) => setStatuses(event.target.value)}
          />
        </Label>
      </div>
      <div className="flex justify-end gap-2">
        <Button
          size="sm"
          variant="ghost"
          disabled={remove.isPending}
          onClick={() =>
            remove.mutate(profile.id, {
              onSuccess: () => toast.success(`Deleted ${profile.name}`),
              onError: (e) =>
                toast.error(e instanceof Error ? e.message : "Delete failed"),
            })
          }
        >
          <Trash2 className="mr-1 h-4 w-4" />
          Delete
        </Button>
        <Button
          size="sm"
          disabled={!dirty || update.isPending}
          onClick={() =>
            update.mutate(
              {
                id: profile.id,
                req: {
                  name,
                  primary_types: splitProfileValues(primary),
                  secondary_types: splitProfileValues(secondary),
                  release_statuses: splitProfileValues(statuses),
                },
              },
              {
                onSuccess: () => toast.success(`Saved ${name}`),
                onError: (e) =>
                  toast.error(e instanceof Error ? e.message : "Save failed"),
              },
            )
          }
        >
          {update.isPending ? "Saving…" : "Save"}
        </Button>
      </div>
    </div>
  );
}

export function MusicProfilesPage() {
  const { data: definitions = [], isLoading: defsLoading } =
    useAudioQualityDefinitions();
  const { data: profiles = [], isLoading: profilesLoading } =
    useAudioQualityProfiles();
  const { data: metadataProfiles = [], isLoading: metaLoading } =
    useMetadataProfiles();
  const createAudioProfile = useCreateAudioQualityProfile();
  const createMetadata = useCreateMetadataProfile();
  const [newAudioName, setNewAudioName] = React.useState("");
  const [newMetadataName, setNewMetadataName] = React.useState("");

  const defName = (id: string) =>
    definitions.find((d) => d.id === id)?.name ?? id;

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-xl font-semibold">Music Profiles</h1>
        <p className="text-sm text-muted-foreground">
          Audio quality tiers and acquisition profiles used when searching for
          music.
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Audio Quality Profiles</CardTitle>
          <CardDescription>
            Which qualities are accepted and the upgrade cutoff.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex gap-2">
            <Input
              aria-label="New audio quality profile name"
              placeholder="New profile name"
              value={newAudioName}
              onChange={(event) => setNewAudioName(event.target.value)}
            />
            <Button
              disabled={!newAudioName.trim() || createAudioProfile.isPending}
              onClick={() =>
                createAudioProfile.mutate(
                  {
                    name: newAudioName.trim(),
                    items: definitions.map((definition) => ({
                      definition_id: definition.id,
                      allowed: true,
                    })),
                    cutoff: definitions.some(
                      (definition) => definition.id === "aq_flac",
                    )
                      ? "aq_flac"
                      : "",
                    upgrade_allowed: true,
                  },
                  {
                    onSuccess: () => {
                      toast.success(`Created ${newAudioName.trim()}`);
                      setNewAudioName("");
                    },
                    onError: (e) =>
                      toast.error(
                        e instanceof Error ? e.message : "Create failed",
                      ),
                  },
                )
              }
            >
              Create profile
            </Button>
          </div>
          {profilesLoading ? (
            <Skeleton className="h-20 w-full" />
          ) : profiles.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No quality profiles configured.
            </p>
          ) : (
            profiles.map((p) => (
              <AudioProfileCard
                key={p.id}
                profile={p}
                definitions={definitions}
                defName={defName}
              />
            ))
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Quality Tiers</CardTitle>
          <CardDescription>
            Known audio qualities, ordered worst to best.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {defsLoading ? (
            <Skeleton className="h-40 w-full" />
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Format</TableHead>
                  <TableHead>Bitrate</TableHead>
                  <TableHead>Lossless</TableHead>
                  <TableHead className="text-right">Tier</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {[...definitions]
                  .sort((a, b) => a.tier_order - b.tier_order)
                  .map((d) => (
                    <TableRow key={d.id}>
                      <TableCell className="font-medium">{d.name}</TableCell>
                      <TableCell className="uppercase">
                        {d.format || "—"}
                      </TableCell>
                      <TableCell>
                        {d.vbr ? "VBR" : d.bitrate ? `${d.bitrate} kbps` : "—"}
                      </TableCell>
                      <TableCell>{d.lossless ? "Yes" : "No"}</TableCell>
                      <TableCell className="text-right tabular-nums">
                        {d.tier_order}
                      </TableCell>
                    </TableRow>
                  ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Metadata Profiles</CardTitle>
          <CardDescription>
            Which release types are monitored when adding artists.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex gap-2">
            <Input
              aria-label="New metadata profile name"
              placeholder="New profile name"
              value={newMetadataName}
              onChange={(event) => setNewMetadataName(event.target.value)}
            />
            <Button
              disabled={!newMetadataName.trim() || createMetadata.isPending}
              onClick={() =>
                createMetadata.mutate(
                  {
                    name: newMetadataName.trim(),
                    primary_types: ["Album", "EP"],
                    secondary_types: [],
                    release_statuses: ["official"],
                  },
                  {
                    onSuccess: () => {
                      toast.success(`Created ${newMetadataName.trim()}`);
                      setNewMetadataName("");
                    },
                    onError: (e) =>
                      toast.error(
                        e instanceof Error ? e.message : "Create failed",
                      ),
                  },
                )
              }
            >
              Create profile
            </Button>
          </div>
          {metaLoading ? (
            <Skeleton className="h-16 w-full" />
          ) : metadataProfiles.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No metadata profiles configured.
            </p>
          ) : (
            metadataProfiles.map((profile) => (
              <MetadataProfileCard key={profile.id} profile={profile} />
            ))
          )}
        </CardContent>
      </Card>
    </div>
  );
}
