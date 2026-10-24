export interface DecisionEvidence {
  field: string;
  expected: string;
  actual: string;
  unit: string;
  passed: boolean;
}

export interface DecisionExplanation {
  category: string;
  summary: string;
  identity?: DecisionEvidence[];
  numbering: {
    season: number;
    episode: number;
    episodes?: number[];
    absolute_episode: number;
    season_pack: boolean;
    daily_date?: string;
    pack_mapping?: string;
  };
  checks?: DecisionEvidence[];
  contributions?: { id: string; name: string; score: number }[];
  rank?: number;
  ranking_reason?: string;
}

export function DecisionDetails({
  explanation,
}: {
  explanation?: DecisionExplanation;
}) {
  if (!explanation) {
    return (
      <p className="text-xs text-muted-foreground">
        Detailed evidence was not recorded for this result.
      </p>
    );
  }
  const numbering = explanation.numbering;
  return (
    <details className="min-w-0 text-sm">
      <summary className="cursor-pointer rounded py-2 focus-visible:outline focus-visible:outline-2">
        {explanation.rank ? `Rank ${explanation.rank}: ` : ""}
        {explanation.category.replaceAll("_", " ")} — decision evidence
      </summary>
      <div className="space-y-3 break-words rounded border border-border bg-muted/30 p-3">
        <p>{explanation.summary}</p>
        {explanation.ranking_reason && <p>{explanation.ranking_reason}</p>}
        <p>
          Parsed season: {numbering.season}; episode: {numbering.episode};
          multi-episode: {numbering.episodes?.join(", ") || "none"}; absolute
          episode: {numbering.absolute_episode}; season pack:{" "}
          {numbering.season_pack ? "yes" : "no"}. Unknown numbering is -1.
          {numbering.daily_date && ` Air date: ${numbering.daily_date}.`}
        </p>
        {numbering.pack_mapping && <p>{numbering.pack_mapping}</p>}
        <p className="text-xs text-muted-foreground">
          Evidence is not a separate decision: an absent embedded ID can be
          corroborated by a title. Checks after the first rejection are not run.
        </p>
        {[...(explanation.identity ?? []), ...(explanation.checks ?? [])].map(
          (check, i) => (
            <div key={`${check.field}-${i}`}>
              <span className="font-medium">{check.field}</span>:{" "}
              {check.passed ? "Matches" : "Not matched / unavailable"}
              <div className="text-xs">
                Expected: {check.expected || "not supplied"}; actual:{" "}
                {check.actual || "not available"}; units: {check.unit}.
              </div>
            </div>
          ),
        )}
        <div>
          <p className="font-medium">Profile-specific format contributions</p>
          {explanation.contributions?.length ? (
            <ul className="list-inside list-disc">
              {explanation.contributions.map((c) => (
                <li key={c.id}>
                  {c.name}: {c.score} points
                </li>
              ))}
            </ul>
          ) : (
            <p>
              No contributions recorded (no matches, or evaluation stopped
              early).
            </p>
          )}
        </div>
      </div>
    </details>
  );
}
