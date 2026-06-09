import * as React from "react";
import { toast } from "sonner";
import {
  Bot,
  Link2,
  Loader2,
  Send,
  Trash2,
  CheckCircle2,
  XCircle,
} from "lucide-react";
import { useSetPageHeader } from "@/hooks/use-page-header";
import { useAuth } from "@/hooks/use-auth";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  useBotConfig,
  useUpdateBotConfig,
  useBotStatus,
  useBotLinks,
  useDeleteBotLink,
  usePreviewLinkCode,
  useRedeemLinkCode,
  type LinkPreview,
  type UpdateBotConfig,
} from "@/lib/bots-api";
import { useQualityProfiles } from "@/lib/quality-profiles-api";
import { useLibraries } from "@/lib/libraries-api";
import { useAudioQualityProfiles } from "@/lib/music-api";
import { useFeatureEnabled } from "@/lib/features-api";

const NONE = "__none__";

export function RequestBotsPage() {
  useSetPageHeader(
    "Request Bots",
    "Let users request media from Telegram & Discord",
  );
  const { user } = useAuth();
  const isAdmin = user?.role === "admin";

  return (
    <div className="mx-auto flex max-w-3xl flex-col gap-6">
      <LinkAccountCard />
      {isAdmin && (
        <>
          <PlatformConfigCard />
          <ApprovalDefaultsCard />
          <StatusCard />
          <LinkedAccountsCard />
        </>
      )}
    </div>
  );
}

// ---------- Link your account (all users) ----------

function LinkAccountCard() {
  const [code, setCode] = React.useState("");
  const [preview, setPreview] = React.useState<LinkPreview | null>(null);
  const previewMut = usePreviewLinkCode();
  const redeemMut = useRedeemLinkCode();

  const onPreview = async () => {
    const trimmed = code.trim();
    if (!trimmed) return;
    try {
      const p = await previewMut.mutateAsync(trimmed);
      setPreview(p);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Invalid code");
    }
  };

  const onConfirm = async () => {
    try {
      const link = await redeemMut.mutateAsync(code.trim());
      toast.success(
        `Linked ${platformLabel(link.platform)} ${link.external_username}`,
      );
      setPreview(null);
      setCode("");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Couldn't link account");
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Link2 className="h-5 w-5" /> Link your chat account
        </CardTitle>
        <CardDescription>
          Send <code className="rounded bg-muted px-1">/link</code> to the bot
          in Telegram or Discord, then enter the code it gives you here to
          connect that chat to your Loom account.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          className="flex items-end gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            void onPreview();
          }}
        >
          <div className="flex-1">
            <Label htmlFor="link-code">Link code</Label>
            <Input
              id="link-code"
              value={code}
              onChange={(e) => setCode(e.target.value.toUpperCase())}
              placeholder="e.g. 7K2P9QXB4M0Z"
              autoComplete="off"
              spellCheck={false}
            />
          </div>
          <Button type="submit" disabled={!code.trim() || previewMut.isPending}>
            {previewMut.isPending ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              "Link"
            )}
          </Button>
        </form>
      </CardContent>

      <Dialog open={!!preview} onOpenChange={(o) => !o && setPreview(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Confirm account link</DialogTitle>
            <DialogDescription>
              {preview && (
                <>
                  Link {platformLabel(preview.platform)} account{" "}
                  <strong>{preview.external_username || "(unknown)"}</strong> to
                  your Loom account? This chat will be able to submit requests
                  as you.
                </>
              )}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPreview(null)}>
              Cancel
            </Button>
            <Button
              onClick={() => void onConfirm()}
              disabled={redeemMut.isPending}
            >
              {redeemMut.isPending ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                "Confirm link"
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  );
}

// ---------- Platform tokens (admin) ----------

function PlatformConfigCard() {
  const { data: cfg, isLoading } = useBotConfig();
  const update = useUpdateBotConfig();

  const [telegramEnabled, setTelegramEnabled] = React.useState(false);
  const [telegramToken, setTelegramToken] = React.useState("");
  const [discordEnabled, setDiscordEnabled] = React.useState(false);
  const [discordToken, setDiscordToken] = React.useState("");
  const seeded = React.useRef(false);

  React.useEffect(() => {
    if (cfg && !seeded.current) {
      setTelegramEnabled(cfg.telegram_enabled);
      setDiscordEnabled(cfg.discord_enabled);
      seeded.current = true;
    }
  }, [cfg]);

  const save = async () => {
    const body: UpdateBotConfig = {
      telegram_enabled: telegramEnabled,
      discord_enabled: discordEnabled,
    };
    if (telegramToken.trim()) body.telegram_bot_token = telegramToken.trim();
    if (discordToken.trim()) body.discord_bot_token = discordToken.trim();
    try {
      await update.mutateAsync(body);
      setTelegramToken("");
      setDiscordToken("");
      toast.success("Bot settings saved");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Couldn't save settings");
    }
  };

  const clearToken = async (platform: "telegram" | "discord") => {
    const body: UpdateBotConfig =
      platform === "telegram"
        ? { telegram_bot_token: "", telegram_enabled: false }
        : { discord_bot_token: "", discord_enabled: false };
    try {
      await update.mutateAsync(body);
      if (platform === "telegram") {
        setTelegramEnabled(false);
        setTelegramToken("");
      } else {
        setDiscordEnabled(false);
        setDiscordToken("");
      }
      toast.success(
        `${platform === "telegram" ? "Telegram" : "Discord"} token cleared`,
      );
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Couldn't clear token");
    }
  };

  if (isLoading || !cfg) return <CardSkeleton />;

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Bot className="h-5 w-5" /> Bot connections
        </CardTitle>
        <CardDescription>
          Create a bot with each platform's developer tools, paste its token
          here, and enable it. Tokens are write-only — saved tokens are never
          shown again.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-6">
        <PlatformRow
          name="Telegram"
          enabled={telegramEnabled}
          onEnabledChange={setTelegramEnabled}
          token={telegramToken}
          onTokenChange={setTelegramToken}
          tokenSet={cfg.telegram_token_set}
          placeholder="123456:ABC-DEF…"
          onClear={() => void clearToken("telegram")}
          clearing={update.isPending}
        />
        <PlatformRow
          name="Discord"
          enabled={discordEnabled}
          onEnabledChange={setDiscordEnabled}
          token={discordToken}
          onTokenChange={setDiscordToken}
          tokenSet={cfg.discord_token_set}
          placeholder="Bot token"
          onClear={() => void clearToken("discord")}
          clearing={update.isPending}
        />
        <div className="flex justify-end">
          <Button onClick={() => void save()} disabled={update.isPending}>
            {update.isPending ? (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : null}
            Save
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

function PlatformRow({
  name,
  enabled,
  onEnabledChange,
  token,
  onTokenChange,
  tokenSet,
  placeholder,
  onClear,
  clearing,
}: {
  name: string;
  enabled: boolean;
  onEnabledChange: (v: boolean) => void;
  token: string;
  onTokenChange: (v: string) => void;
  tokenSet: boolean;
  placeholder: string;
  onClear: () => void;
  clearing: boolean;
}) {
  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center justify-between">
        <Label className="text-base">{name}</Label>
        <div className="flex items-center gap-2">
          {tokenSet ? (
            <>
              <Badge variant="secondary">Token set</Badge>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-7 px-2 text-xs text-muted-foreground"
                onClick={onClear}
                disabled={clearing}
              >
                Clear
              </Button>
            </>
          ) : (
            <Badge variant="outline">No token</Badge>
          )}
          <Switch checked={enabled} onCheckedChange={onEnabledChange} />
        </div>
      </div>
      <Input
        type="password"
        value={token}
        onChange={(e) => onTokenChange(e.target.value)}
        placeholder={tokenSet ? "•••••••• (leave blank to keep)" : placeholder}
        autoComplete="off"
      />
    </div>
  );
}

// ---------- Chat-approval defaults (admin) ----------

function ApprovalDefaultsCard() {
  const { data: cfg, isLoading } = useBotConfig();
  const update = useUpdateBotConfig();
  const { data: profiles } = useQualityProfiles();
  const { data: libraries } = useLibraries();
  const { data: audioProfiles } = useAudioQualityProfiles();
  const musicEnabled = useFeatureEnabled("music", false);

  const [movieQP, setMovieQP] = React.useState("");
  const [movieLib, setMovieLib] = React.useState("");
  const [seriesQP, setSeriesQP] = React.useState("");
  const [seriesLib, setSeriesLib] = React.useState("");
  const [musicQP, setMusicQP] = React.useState("");
  const [musicLib, setMusicLib] = React.useState("");
  const seeded = React.useRef(false);

  React.useEffect(() => {
    if (cfg && !seeded.current) {
      setMovieQP(cfg.default_movie_quality_profile_id);
      setMovieLib(cfg.default_movie_library_id);
      setSeriesQP(cfg.default_series_quality_profile_id);
      setSeriesLib(cfg.default_series_library_id);
      setMusicQP(cfg.default_music_quality_profile_id);
      setMusicLib(cfg.default_music_library_id);
      seeded.current = true;
    }
  }, [cfg]);

  const movieLibs = (libraries ?? []).filter((l) => l.media_type === "movie");
  const seriesLibs = (libraries ?? []).filter((l) => l.media_type === "series");
  const musicLibs = (libraries ?? []).filter((l) => l.media_type === "music");

  const save = async () => {
    try {
      await update.mutateAsync({
        default_movie_quality_profile_id: movieQP,
        default_movie_library_id: movieLib,
        default_series_quality_profile_id: seriesQP,
        default_series_library_id: seriesLib,
        default_music_quality_profile_id: musicQP,
        default_music_library_id: musicLib,
      });
      toast.success("Approval defaults saved");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Couldn't save defaults");
    }
  };

  if (isLoading || !cfg) return <CardSkeleton />;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Chat-approval defaults</CardTitle>
        <CardDescription>
          When an admin approves a request from chat, these targets are used
          (chat has no picker like the web UI). Required to approve from chat.
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 sm:grid-cols-2">
        <SelectField
          label="Movie quality profile"
          value={movieQP}
          onChange={setMovieQP}
          options={(profiles ?? []).map((p) => ({
            value: p.id,
            label: p.name,
          }))}
        />
        <SelectField
          label="Movie library"
          value={movieLib}
          onChange={setMovieLib}
          options={movieLibs.map((l) => ({ value: l.id, label: l.name }))}
        />
        <SelectField
          label="Series quality profile"
          value={seriesQP}
          onChange={setSeriesQP}
          options={(profiles ?? []).map((p) => ({
            value: p.id,
            label: p.name,
          }))}
        />
        <SelectField
          label="Series library"
          value={seriesLib}
          onChange={setSeriesLib}
          options={seriesLibs.map((l) => ({ value: l.id, label: l.name }))}
        />
        {musicEnabled && (
          <>
            <SelectField
              label="Music quality profile"
              value={musicQP}
              onChange={setMusicQP}
              options={(audioProfiles ?? []).map((p) => ({
                value: p.id,
                label: p.name,
              }))}
            />
            <SelectField
              label="Music library"
              value={musicLib}
              onChange={setMusicLib}
              options={musicLibs.map((l) => ({ value: l.id, label: l.name }))}
            />
          </>
        )}
        <div className="flex justify-end sm:col-span-2">
          <Button onClick={() => void save()} disabled={update.isPending}>
            {update.isPending ? (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : null}
            Save defaults
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

function SelectField({
  label,
  value,
  onChange,
  options,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  options: { value: string; label: string }[];
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label>{label}</Label>
      <Select
        value={value || NONE}
        onValueChange={(v) => onChange(v === NONE ? "" : v)}
      >
        <SelectTrigger>
          <SelectValue placeholder="Not set" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={NONE}>Not set</SelectItem>
          {options.map((o) => (
            <SelectItem key={o.value} value={o.value}>
              {o.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

// ---------- Status (admin) ----------

function StatusCard() {
  const { data: statuses, isLoading } = useBotStatus();
  if (isLoading) return <CardSkeleton />;
  return (
    <Card>
      <CardHeader>
        <CardTitle>Connection status</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {(statuses ?? []).map((s) => (
          <div key={s.platform} className="flex items-center justify-between">
            <span className="font-medium">{platformLabel(s.platform)}</span>
            <div className="flex items-center gap-2">
              {s.last_error ? (
                <span className="text-sm text-destructive">{s.last_error}</span>
              ) : null}
              {s.running ? (
                <Badge variant="secondary" className="gap-1">
                  <CheckCircle2 className="h-3.5 w-3.5" /> Running
                </Badge>
              ) : (
                <Badge variant="outline" className="gap-1">
                  <XCircle className="h-3.5 w-3.5" /> Stopped
                </Badge>
              )}
            </div>
          </div>
        ))}
      </CardContent>
    </Card>
  );
}

// ---------- Linked accounts (admin) ----------

function LinkedAccountsCard() {
  const { data: links, isLoading } = useBotLinks();
  const del = useDeleteBotLink();

  const unlink = async (id: string) => {
    try {
      await del.mutateAsync(id);
      toast.success("Account unlinked");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Couldn't unlink");
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Send className="h-5 w-5" /> Linked accounts
        </CardTitle>
        <CardDescription>Chat identities bound to Loom users.</CardDescription>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <Skeleton className="h-16 w-full" />
        ) : (links ?? []).length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No linked accounts yet.
          </p>
        ) : (
          <div className="flex flex-col divide-y">
            {(links ?? []).map((l) => (
              <div
                key={l.id}
                className="flex items-center justify-between py-2"
              >
                <div className="flex flex-col">
                  <span className="font-medium">
                    {l.external_username || l.external_id}{" "}
                    <Badge variant="outline" className="ml-1">
                      {platformLabel(l.platform)}
                    </Badge>
                  </span>
                  <span className="text-sm text-muted-foreground">
                    Loom user: {l.username || `#${l.user_id}`}
                  </span>
                </div>
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={() => void unlink(l.id)}
                  disabled={del.isPending}
                  aria-label="Unlink"
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

// ---------- helpers ----------

function platformLabel(p: string) {
  return p === "telegram" ? "Telegram" : p === "discord" ? "Discord" : p;
}

function CardSkeleton() {
  return (
    <Card>
      <CardHeader>
        <Skeleton className="h-5 w-40" />
      </CardHeader>
      <CardContent>
        <Skeleton className="h-24 w-full" />
      </CardContent>
    </Card>
  );
}
