import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
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
  type AudioQualityProfile,
} from "@/lib/music-api";

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
            profiles.map((p) => {
              const items = parseItems(p).filter((i) => i.allowed);
              return (
                <div key={p.id} className="rounded-md border p-3">
                  <div className="mb-2 flex items-center justify-between">
                    <span className="font-medium">{p.name}</span>
                    <span className="text-xs text-muted-foreground">
                      Cutoff: {p.cutoff ? defName(p.cutoff) : "—"}
                      {p.upgrade_allowed ? " · upgrades on" : " · upgrades off"}
                    </span>
                  </div>
                  <div className="flex flex-wrap gap-1.5">
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
                </div>
              );
            })
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
