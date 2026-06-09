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
  useMetadataProfiles,
  useUpdateAudioQualityProfile,
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
  defName,
}: {
  profile: AudioQualityProfile;
  defName: (id: string) => string;
}) {
  const items = parseItems(profile).filter((i) => i.allowed);
  const { data: customFormats = [] } = useCustomFormats();
  const update = useUpdateAudioQualityProfile();

  const initialScores = React.useMemo(() => {
    const m: Record<string, number> = {};
    for (const fi of profile.format_items ?? []) m[fi.format_id] = fi.score;
    return m;
  }, [profile.format_items]);

  const [scores, setScores] = React.useState<Record<string, number>>(
    initialScores,
  );
  const [minScore, setMinScore] = React.useState<number>(
    profile.min_format_score ?? 0,
  );

  React.useEffect(() => {
    setScores(initialScores);
    setMinScore(profile.min_format_score ?? 0);
  }, [initialScores, profile.min_format_score]);

  const dirty =
    minScore !== (profile.min_format_score ?? 0) ||
    customFormats.some((cf) => (scores[cf.id] ?? 0) !== (initialScores[cf.id] ?? 0));

  const save = () => {
    const format_items = customFormats
      .map((cf) => ({ format_id: cf.id, score: scores[cf.id] ?? 0 }))
      .filter((fi) => fi.score !== 0);
    update.mutate(
      { id: profile.id, req: { format_items, min_format_score: minScore } },
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
        <span className="text-xs text-muted-foreground">
          Cutoff: {profile.cutoff ? defName(profile.cutoff) : "—"}
          {profile.upgrade_allowed ? " · upgrades on" : " · upgrades off"}
        </span>
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

export function MusicProfilesPage() {
  const { data: definitions = [], isLoading: defsLoading } =
    useAudioQualityDefinitions();
  const { data: profiles = [], isLoading: profilesLoading } =
    useAudioQualityProfiles();
  const { data: metadataProfiles = [], isLoading: metaLoading } =
    useMetadataProfiles();

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
          {profilesLoading ? (
            <Skeleton className="h-20 w-full" />
          ) : profiles.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No quality profiles configured.
            </p>
          ) : (
            profiles.map((p) => (
              <AudioProfileCard key={p.id} profile={p} defName={defName} />
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
                        {d.vbr
                          ? "VBR"
                          : d.bitrate
                            ? `${d.bitrate} kbps`
                            : "—"}
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
          {metaLoading ? (
            <Skeleton className="h-16 w-full" />
          ) : metadataProfiles.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No metadata profiles configured.
            </p>
          ) : (
            metadataProfiles.map((m) => (
              <div key={m.id} className="rounded-md border p-3">
                <div className="mb-1 font-medium">{m.name}</div>
                <div className="flex flex-wrap gap-1.5">
                  {(m.primary_types ?? []).map((t) => (
                    <Badge key={t} variant="secondary">
                      {t}
                    </Badge>
                  ))}
                  {(m.secondary_types ?? []).map((t) => (
                    <Badge key={t} variant="outline">
                      {t}
                    </Badge>
                  ))}
                </div>
              </div>
            ))
          )}
        </CardContent>
      </Card>
    </div>
  );
}
