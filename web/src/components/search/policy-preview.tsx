import { useId, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  evaluateResults,
  type EvaluateParams,
  type SearchResult,
} from "@/lib/indexers-api";
import { useQualityProfiles } from "@/lib/quality-profiles-api";
import { DecisionDetails } from "./decision-explanation";

export function PolicyPreview({
  context,
  results,
}: {
  context: Omit<EvaluateParams, "results" | "policy_preview">;
  results: SearchResult[];
}) {
  const id = useId();
  const profiles = useQualityProfiles();
  const [selected, setSelected] = useState<string | null>(null);
  const [minimum, setMinimum] = useState("");
  const profileID = selected ?? context.quality_profile_id;
  const profile = profiles.data?.find((p) => p.id === profileID);
  const isPreview = minimum !== "" || profileID !== context.quality_profile_id;
  const validMinimum = minimum === "" || Number.isSafeInteger(Number(minimum));
  const params: EvaluateParams = {
    ...context,
    quality_profile_id: profileID,
    results,
    ...(isPreview && profile
      ? {
          policy_preview: {
            items: profile.items,
            cutoff: profile.cutoff,
            upgrade_allowed: profile.upgrade_allowed,
            min_format_score:
              minimum === ""
                ? (profile.min_format_score ?? 0)
                : Number(minimum),
            format_items: (profile.format_items ?? []).map((fi) => ({
              format_id: fi.format_id ?? fi.custom_format_id,
              score: fi.score,
            })),
          },
        }
      : {}),
  };
  const evaluation = useQuery({
    queryKey: ["release-policy-evaluation", params],
    queryFn: () => evaluateResults(params),
    enabled:
      results.length > 0 &&
      !!profileID &&
      validMinimum &&
      (!isPreview || !!profile),
    retry: false,
  });

  return (
    <details className="min-h-10 shrink-0 overflow-auto rounded border border-border p-3 text-sm">
      <summary className="cursor-pointer font-medium focus-visible:outline focus-visible:outline-2">
        Explain matching / preview policy
      </summary>
      <section
        aria-label="Release policy evaluation"
        className="max-h-80 space-y-3 overflow-auto pt-3"
      >
        <div className="flex flex-wrap items-end gap-3">
          <div className="space-y-1">
            <label htmlFor={`${id}-profile`}>Evaluate quality profile</label>
            <select
              id={`${id}-profile`}
              className="block min-h-10 max-w-full rounded border border-input bg-background p-2"
              value={profileID}
              onChange={(e) => {
                setSelected(e.target.value);
                setMinimum("");
              }}
            >
              <option value="">Select a profile</option>
              {profileID && !profile && (
                <option value={profileID}>
                  Assigned profile ({profileID})
                </option>
              )}
              {profiles.data?.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
          </div>
          <div className="space-y-1">
            <label htmlFor={`${id}-minimum`}>
              Preview minimum format score (points)
            </label>
            <Input
              id={`${id}-minimum`}
              type="number"
              step={1}
              value={minimum}
              placeholder={`Saved: ${profile?.min_format_score ?? 0}`}
              onChange={(e) => setMinimum(e.target.value)}
              disabled={!profile}
              aria-invalid={!validMinimum}
            />
          </div>
          <Button
            variant="outline"
            disabled={!isPreview}
            onClick={() => {
              setSelected(null);
              setMinimum("");
            }}
          >
            Reset to assigned policy
          </Button>
        </div>
        <p className="text-xs text-muted-foreground">
          {isPreview
            ? "Preview only — no profile is saved and no download is started. "
            : "Saved-policy dry run — no download is started. "}
          For example, a preferred quality tier wins before custom-format
          points; raising the minimum from 0 to 100 rejects a release with 50
          points. Policy cannot override wrong identity or map absolute anime
          numbers to a season.
        </p>
        {!validMinimum && <p role="alert">Enter a whole number of points.</p>}
        {profiles.isError && (
          <p role="alert">
            Profiles could not be loaded: {profiles.error.message}
          </p>
        )}
        {!profileID && <p>Select a profile to see actual-engine decisions.</p>}
        {evaluation.isFetching && (
          <p role="status">Evaluating releases with the production engine…</p>
        )}
        {evaluation.isError && (
          <p role="alert">Evaluation failed: {evaluation.error.message}</p>
        )}
        {!evaluation.isFetching && !evaluation.isError && evaluation.data && (
          <div className="max-h-64 space-y-3 overflow-auto" aria-live="polite">
            <p>
              {evaluation.data.preview ? "Preview" : "Saved policy"}:{" "}
              {evaluation.data.passed} of {evaluation.data.total} accepted.
              Ordered by the engine, not the raw search score.
            </p>
            {evaluation.data.results.map((r, i) => (
              <div
                key={`${r.indexer_id}-${r.link}-${i}`}
                className="border-t border-border pt-2"
              >
                <p className="break-words font-medium">{r.title}</p>
                <p>
                  {r.rejected ? `Rejected: ${r.reject_reason}` : "Accepted"};
                  quality: {r.quality_name || "not evaluated"}; tier:{" "}
                  {r.quality_tier}; format: {r.format_score} points.
                </p>
                <DecisionDetails explanation={r.explanation} />
              </div>
            ))}
          </div>
        )}
      </section>
    </details>
  );
}
