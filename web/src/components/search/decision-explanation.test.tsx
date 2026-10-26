import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import {
  DecisionDetails,
  type DecisionExplanation,
} from "./decision-explanation";

describe("DecisionDetails", () => {
  it("keeps historical records without explanation readable", () => {
    render(<DecisionDetails />);
    expect(
      screen.getByText(/Detailed evidence was not recorded/),
    ).toBeInTheDocument();
  });

  it("renders engine evidence and units without deciding again", () => {
    const explanation: DecisionExplanation = {
      category: "configuration_mismatch",
      summary: "Release exceeds the maximum size.",
      numbering: {
        season: 2,
        episode: -1,
        absolute_episode: -1,
        season_pack: true,
        pack_mapping: "No correction applied.",
      },
      checks: [
        {
          field: "maximum size",
          expected: "<= 1000",
          actual: "1001",
          unit: "bytes",
          passed: false,
        },
      ],
      contributions: [{ id: "preferred", name: "Preferred group", score: -50 }],
      ranking_reason: "Composite score is display-only.",
    };
    render(<DecisionDetails explanation={explanation} />);
    expect(screen.getByText(/configuration mismatch/)).toBeInTheDocument();
    expect(
      screen.getByText(/Expected: <= 1000; actual: 1001; units: bytes/),
    ).toBeInTheDocument();
    expect(screen.getByText("Preferred group: -50 points")).toBeInTheDocument();
    expect(screen.getByText("No correction applied.")).toBeInTheDocument();
  });
});
