import { useState, useEffect, useCallback } from "react";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Save, RotateCcw, FolderOpen, FileText } from "lucide-react";
import { toast } from "sonner";

interface NamingConfig {
  movie_folder_format: string;
  movie_file_format: string;
  colon_replacement: string;
  rename_movies: boolean;
}

interface PreviewSample {
  folder_example: string;
  file_example: string;
  full_path: string;
}

const TOKENS = [
  { token: "{Movie Title}", desc: "Full movie title" },
  { token: "{Movie CleanTitle}", desc: "Title without articles or special chars" },
  { token: "{Movie TitleThe}", desc: 'Title with article moved to end (e.g. "Matrix, The")' },
  { token: "{Release Year}", desc: "Year of release" },
  { token: "{Quality Full}", desc: "Full quality string (e.g. Bluray-1080p)" },
  { token: "{Quality Resolution}", desc: "Resolution only (e.g. 1080p)" },
  { token: "{Quality Source}", desc: "Source only (e.g. BluRay)" },
  { token: "{MediaInfo VideoCodec}", desc: "Video codec (e.g. x264)" },
  { token: "{MediaInfo AudioCodec}", desc: "Audio codec (e.g. DTS-HD MA)" },
  { token: "{MediaInfo AudioChannels}", desc: "Audio channels (e.g. 5.1)" },
  { token: "{MediaInfo VideoDynamicRange}", desc: "HDR/SDR" },
  { token: "{IMDB Id}", desc: "IMDB identifier" },
  { token: "{TMDB Id}", desc: "TMDB identifier" },
];

const COLON_OPTIONS = [
  { value: " -", label: "Replace with dash ( -)" },
  { value: " ", label: "Replace with space" },
  { value: "", label: "Remove" },
  { value: "꞉", label: "Smart quotes (꞉)" },
];

const DEFAULTS: NamingConfig = {
  movie_folder_format: "{Movie Title} ({Release Year})",
  movie_file_format: "{Movie Title} ({Release Year}) [{Quality Full}]",
  colon_replacement: " -",
  rename_movies: true,
};

export function NamingSettings() {
  const [config, setConfig] = useState<NamingConfig>(DEFAULTS);
  const [preview, setPreview] = useState<PreviewSample | null>(null);
  const [saving, setSaving] = useState(false);
  const [dirty, setDirty] = useState(false);

  // Load current config
  useEffect(() => {
    fetch("/api/v1/movies/organize/naming")
      .then((r) => r.json())
      .then((data: NamingConfig) => {
        setConfig(data);
        fetchPreview(data);
      })
      .catch(() => {
        // Use defaults if not configured yet
        fetchPreview(DEFAULTS);
      });
  }, []);

  const fetchPreview = useCallback(async (cfg: NamingConfig) => {
    try {
      const res = await fetch("/api/v1/movies/organize/naming/preview", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(cfg),
      });
      if (res.ok) {
        setPreview(await res.json());
      }
    } catch {
      // Silent fail for preview
    }
  }, []);

  const updateField = <K extends keyof NamingConfig>(
    key: K,
    value: NamingConfig[K]
  ) => {
    const updated = { ...config, [key]: value };
    setConfig(updated);
    setDirty(true);
    // Debounced preview
    fetchPreview(updated);
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      const res = await fetch("/api/v1/movies/organize/naming", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(config),
      });
      if (!res.ok) throw new Error(await res.text());
      setDirty(false);
      toast.success("Naming settings saved");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Failed to save");
    } finally {
      setSaving(false);
    }
  };

  const handleReset = () => {
    setConfig(DEFAULTS);
    setDirty(true);
    fetchPreview(DEFAULTS);
  };

  const insertToken = (field: "movie_folder_format" | "movie_file_format", token: string) => {
    updateField(field, config[field] + token);
  };

  return (
    <Card className="p-6 bg-zinc-900/50 border-zinc-800 space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-lg font-semibold text-zinc-100">Movie Naming</h3>
          <p className="text-sm text-zinc-500 mt-1">
            Configure how movie files and folders are named when organized.
          </p>
        </div>
        <div className="flex gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={handleReset}
            className="border-zinc-700 text-zinc-400"
          >
            <RotateCcw className="h-3 w-3 mr-1" /> Reset
          </Button>
          <Button
            size="sm"
            onClick={handleSave}
            disabled={!dirty || saving}
            className="bg-teal-600 hover:bg-teal-700"
          >
            <Save className="h-3 w-3 mr-1" /> Save
          </Button>
        </div>
      </div>

      {/* Rename toggle */}
      <div className="flex items-center gap-3">
        <Checkbox
          id="rename-movies"
          checked={config.rename_movies}
          onCheckedChange={(checked) =>
            updateField("rename_movies", checked === true)
          }
        />
        <Label htmlFor="rename-movies" className="text-sm text-zinc-300">
          Rename movies on import and when manually organizing
        </Label>
      </div>

      {/* Colon replacement */}
      <div className="space-y-2">
        <Label className="text-sm text-zinc-400">Colon Replacement</Label>
        <Select
          value={config.colon_replacement}
          onValueChange={(v) => updateField("colon_replacement", v)}
        >
          <SelectTrigger className="w-64 bg-zinc-900 border-zinc-700">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {COLON_OPTIONS.map((opt) => (
              <SelectItem key={opt.value} value={opt.value || "__empty__"}>
                {opt.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {/* Folder format */}
      <div className="space-y-2">
        <Label className="text-sm text-zinc-400 flex items-center gap-2">
          <FolderOpen className="h-4 w-4" /> Movie Folder Format
        </Label>
        <Input
          value={config.movie_folder_format}
          onChange={(e) => updateField("movie_folder_format", e.target.value)}
          className="bg-zinc-900 border-zinc-700 font-mono text-sm"
        />
        <div className="flex flex-wrap gap-1">
          {TOKENS.slice(0, 5).map((t) => (
            <Button
              key={t.token}
              variant="outline"
              size="sm"
              className="text-xs border-zinc-700 text-zinc-400 h-6 px-2"
              onClick={() => insertToken("movie_folder_format", t.token)}
              title={t.desc}
            >
              {t.token}
            </Button>
          ))}
        </div>
      </div>

      {/* File format */}
      <div className="space-y-2">
        <Label className="text-sm text-zinc-400 flex items-center gap-2">
          <FileText className="h-4 w-4" /> Movie File Format
        </Label>
        <Input
          value={config.movie_file_format}
          onChange={(e) => updateField("movie_file_format", e.target.value)}
          className="bg-zinc-900 border-zinc-700 font-mono text-sm"
        />
        <div className="flex flex-wrap gap-1">
          {TOKENS.map((t) => (
            <Button
              key={t.token}
              variant="outline"
              size="sm"
              className="text-xs border-zinc-700 text-zinc-400 h-6 px-2"
              onClick={() => insertToken("movie_file_format", t.token)}
              title={t.desc}
            >
              {t.token}
            </Button>
          ))}
        </div>
      </div>

      {/* Live preview */}
      {preview && (
        <div className="space-y-2 p-4 rounded-lg bg-zinc-950 border border-zinc-800">
          <p className="text-xs font-medium text-zinc-500 uppercase tracking-wide">
            Preview (Sample: The Dark Knight, 2008)
          </p>
          <div className="space-y-1">
            <div className="flex items-center gap-2">
              <FolderOpen className="h-3.5 w-3.5 text-teal-400 flex-shrink-0" />
              <span className="text-sm font-mono text-zinc-300">
                {preview.folder_example}/
              </span>
            </div>
            <div className="flex items-center gap-2 ml-5">
              <FileText className="h-3.5 w-3.5 text-zinc-500 flex-shrink-0" />
              <span className="text-sm font-mono text-zinc-400">
                {preview.file_example}
              </span>
            </div>
          </div>
          <div className="mt-2 pt-2 border-t border-zinc-800">
            <Badge variant="outline" className="text-xs border-zinc-700 text-zinc-500 font-mono">
              {preview.full_path}
            </Badge>
          </div>
        </div>
      )}
    </Card>
  );
}
