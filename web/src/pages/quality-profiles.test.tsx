import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { QualityProfilesPage } from "./quality-profiles";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const definitions = [
  {
    id: "web-1080p",
    name: "WEB-1080p",
    title: "WEB 1080p",
    source: "Web",
    resolution: "1080p",
    preferred_at: 1,
  },
  {
    id: "bluray-1080p",
    name: "BluRay-1080p",
    title: "Blu-ray 1080p",
    source: "BluRay",
    resolution: "1080p",
    preferred_at: 2,
  },
];
const formats = [
  { id: "preferred", name: "Preferred group", specifications: [] },
  { id: "avoid", name: "Avoid group", specifications: [] },
  { id: "neutral", name: "Neutral format", specifications: [] },
];
const initialProfile = {
  id: "profile",
  name: "Test profile",
  cutoff: "web-1080p",
  items: JSON.stringify(
    definitions.map((d) => ({
      id: d.id,
      name: d.name,
      allowed: d.id === "web-1080p",
      preferred: d.id === "web-1080p",
    })),
  ),
  upgrade_allowed: true,
  min_format_score: 25,
  format_items: [
    { format_id: "preferred", score: 100 },
    { format_id: "avoid", score: -50 },
    { format_id: "neutral", score: 0 },
    { format_id: "unavailable-format", score: 17 },
  ],
};
let savedProfile = structuredClone(initialProfile);
let savedTiers: Array<{
  id: string;
  name: string;
  specifications: Array<{
    name: string;
    implementation: string;
    negate: boolean;
    required: boolean;
    fields: Record<string, unknown>;
  }>;
}> = [];
const fetchMock = vi.fn<typeof fetch>();
const json = (body: unknown) =>
  new Response(JSON.stringify(body), {
    headers: { "Content-Type": "application/json" },
  });

function show() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  render(
    <QueryClientProvider client={client}>
      <QualityProfilesPage />
    </QueryClientProvider>,
  );
}

async function openEditor(user: ReturnType<typeof userEvent.setup>) {
  await user.click(
    await screen.findByRole("button", { name: "Actions for Test profile" }),
  );
  await user.click(screen.getByRole("menuitem", { name: "Edit" }));
  await screen.findByLabelText("Preferred group score (points)");
}

beforeEach(() => {
  savedProfile = structuredClone(initialProfile);
  savedTiers = [];
  vi.stubGlobal("fetch", fetchMock);
  fetchMock.mockImplementation(async (input, init) => {
    const url = String(input);
    if (url.endsWith("/movies/quality-definitions")) return json(definitions);
    if (url.endsWith("/custom-formats/tiers") && init?.method === "POST") {
      const tier = { ...JSON.parse(String(init.body)), id: "ct_test" };
      savedTiers.push(tier);
      return json(tier);
    }
    if (
      url.endsWith("/custom-formats/tiers/ct_test") &&
      init?.method === "PUT"
    ) {
      savedTiers[0] = JSON.parse(String(init.body));
      return json(savedTiers[0]);
    }
    if (
      url.endsWith("/custom-formats/tiers/ct_test") &&
      init?.method === "DELETE"
    ) {
      savedTiers = [];
      return new Response(null, { status: 204 });
    }
    if (url.endsWith("/custom-formats/tiers"))
      return json({ data: savedTiers });
    if (url.endsWith("/custom-formats")) return json({ data: formats });
    if (url.endsWith("/quality-profiles/profile") && init?.method === "PUT") {
      savedProfile = { ...savedProfile, ...JSON.parse(String(init.body)) };
      return json(savedProfile);
    }
    if (url.endsWith("/quality-profiles") && init?.method === "POST") {
      return json({ id: "new-profile", ...JSON.parse(String(init.body)) });
    }
    if (url.endsWith("/quality-profiles"))
      return json({ data: [savedProfile] });
    throw new Error(`Unexpected request: ${init?.method} ${url}`);
  });
});

afterEach(() => {
  vi.unstubAllGlobals();
  fetchMock.mockReset();
});

describe("Quality profile scoring", () => {
  it("edits and deletes an unreferenced custom tier", async () => {
    savedTiers = [
      {
        id: "ct_test",
        name: "Original tier",
        specifications: [
          {
            name: "",
            implementation: "CodecSpec",
            negate: false,
            required: false,
            fields: { value: "x265" },
          },
        ],
      },
    ];
    vi.spyOn(window, "confirm").mockReturnValue(true);
    const user = userEvent.setup();
    show();
    await user.click(
      await screen.findByRole("button", { name: "Manage custom tiers" }),
    );
    await user.click(
      await screen.findByRole("button", { name: "Edit Original tier" }),
    );
    await user.clear(screen.getByLabelText("Tier name"));
    await user.type(screen.getByLabelText("Tier name"), "Renamed tier");
    await user.click(screen.getByRole("button", { name: "Save tier" }));
    await user.click(
      await screen.findByRole("button", { name: "Delete Renamed tier" }),
    );
    await waitFor(() => expect(savedTiers).toHaveLength(0));
    vi.restoreAllMocks();
  });
  it("creates a matcher-backed tier and saves its profile position", async () => {
    const user = userEvent.setup();
    show();
    await user.click(
      await screen.findByRole("button", { name: "Manage custom tiers" }),
    );
    await user.click(screen.getByRole("button", { name: "Add custom tier" }));
    await user.type(
      screen.getByLabelText("Tier name"),
      "1080p HEVC unknown source",
    );
    await user.type(
      screen.getByLabelText("Value", { selector: "#tier-value-0" }),
      "1080p",
    );
    await user.click(screen.getByRole("button", { name: "Add rule" }));
    await user.selectOptions(
      screen.getByLabelText("Rule type", { selector: "#tier-type-1" }),
      "CodecSpec",
    );
    await user.type(
      screen.getByLabelText("Value", { selector: "#tier-value-1" }),
      "x265",
    );
    await user.click(screen.getByRole("button", { name: "Add rule" }));
    await user.selectOptions(
      screen.getByLabelText("Rule type", { selector: "#tier-type-2" }),
      "SourceSpec",
    );
    await user.type(
      screen.getByLabelText("Value", { selector: "#tier-value-2" }),
      "unknown",
    );
    await user.click(screen.getByRole("button", { name: "Save tier" }));
    await screen.findByRole("button", {
      name: "Edit 1080p HEVC unknown source",
    });
    await user.keyboard("{Escape}");
    await openEditor(user);
    await user.click(
      screen.getByText("1080p HEVC unknown source", {
        selector: "span.text-sm.text-foreground",
      }),
    );
    await user.click(
      screen.getByRole("button", { name: "Move 1080p HEVC unknown source up" }),
    );
    await user.click(
      screen.getByRole("button", { name: "Move 1080p HEVC unknown source up" }),
    );
    await user.selectOptions(
      screen.getByLabelText("Quality cutoff"),
      "ct_test",
    );
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(savedProfile.cutoff).toBe("ct_test"));
    expect(JSON.parse(savedProfile.items)[0]).toMatchObject({
      id: "ct_test",
      kind: "custom",
      allowed: true,
    });
  });
  it("preserves scores, format IDs and thresholds through quality selection, save and reopen", async () => {
    const user = userEvent.setup();
    show();
    await openEditor(user);
    expect(screen.getByLabelText("Preferred group score (points)")).toHaveValue(
      100,
    );
    expect(screen.getByLabelText("Avoid group score (points)")).toHaveValue(
      -50,
    );
    expect(screen.getByLabelText("Neutral format score (points)")).toHaveValue(
      0,
    );
    expect(
      screen.getByLabelText("Minimum custom format score (points)"),
    ).toHaveValue(25);
    expect(screen.getByLabelText("Quality cutoff")).toHaveValue("web-1080p");

    await user.click(screen.getByRole("button", { name: "Select All" }));
    await user.click(screen.getByRole("button", { name: "Deselect All" }));
    expect(screen.queryByLabelText("Quality cutoff")).not.toBeInTheDocument();
    expect(screen.getByLabelText("Avoid group score (points)")).toHaveValue(
      -50,
    );
    await user.click(screen.getByRole("button", { name: "Select All" }));
    await user.selectOptions(
      screen.getByLabelText("Quality cutoff"),
      "bluray-1080p",
    );

    await user.clear(screen.getByLabelText("Avoid group score (points)"));
    await user.type(screen.getByLabelText("Avoid group score (points)"), "-75");
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
    expect(savedProfile.format_items).toEqual([
      initialProfile.format_items[0],
      { format_id: "avoid", score: -75 },
      initialProfile.format_items[2],
      initialProfile.format_items[3],
    ]);
    expect(savedProfile.min_format_score).toBe(25);
    expect(
      JSON.parse(savedProfile.items).every(
        (item: { allowed: boolean }) => item.allowed,
      ),
    ).toBe(true);

    await openEditor(user);
    expect(screen.getByLabelText("Avoid group score (points)")).toHaveValue(
      -75,
    );
    expect(screen.getByLabelText("Quality cutoff")).toHaveValue("bluray-1080p");
    expect(
      screen.getByLabelText("Minimum custom format score (points)"),
    ).toHaveValue(25);
    await user.clear(
      screen.getByLabelText("Minimum custom format score (points)"),
    );
    await user.type(
      screen.getByLabelText("Minimum custom format score (points)"),
      "50",
    );
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(savedProfile.min_format_score).toBe(50));
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
    await openEditor(user);
    expect(
      screen.getByLabelText("Minimum custom format score (points)"),
    ).toHaveValue(50);
  });

  it("discards unsaved edits without mutating saved scores", async () => {
    const user = userEvent.setup();
    show();
    await openEditor(user);
    await user.clear(screen.getByLabelText("Preferred group score (points)"));
    await user.type(
      screen.getByLabelText("Preferred group score (points)"),
      "900",
    );
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    await openEditor(user);
    expect(screen.getByLabelText("Preferred group score (points)")).toHaveValue(
      100,
    );
    expect(savedProfile).toEqual(initialProfile);
  });

  it("creates a profile with two canonical format IDs, signed scores and a minimum", async () => {
    const user = userEvent.setup();
    show();
    await user.click(screen.getByRole("button", { name: "Add Profile" }));
    await user.type(screen.getByLabelText("Profile Name"), "New profile");
    await user.click(
      await screen.findByRole("checkbox", { name: /WEB 1080p/ }),
    );
    await user.type(
      screen.getByLabelText("Preferred group score (points)"),
      "100",
    );
    await user.clear(screen.getByLabelText("Avoid group score (points)"));
    await user.type(screen.getByLabelText("Avoid group score (points)"), "-25");
    await user.type(
      screen.getByLabelText("Minimum custom format score (points)"),
      "50",
    );
    await user.click(screen.getByRole("button", { name: "Create" }));
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
    const request = fetchMock.mock.calls.find(
      ([, init]) => init?.method === "POST",
    );
    const body = JSON.parse(String(request?.[1]?.body));
    expect(body).toMatchObject({
      name: "New profile",
      min_format_score: 50,
      format_items: [
        { format_id: "preferred", score: 100 },
        { format_id: "avoid", score: -25 },
      ],
    });
    expect(JSON.parse(body.items)).toEqual([
      expect.objectContaining({ id: "web-1080p", allowed: true }),
      expect.objectContaining({ id: "bluray-1080p", allowed: false }),
    ]);
  });

  it("adds multiple scores to an existing profile using the API field names", async () => {
    savedProfile.format_items = [];
    const user = userEvent.setup();
    show();
    await openEditor(user);
    await user.clear(screen.getByLabelText("Preferred group score (points)"));
    await user.type(
      screen.getByLabelText("Preferred group score (points)"),
      "100",
    );
    await user.clear(screen.getByLabelText("Avoid group score (points)"));
    await user.type(screen.getByLabelText("Avoid group score (points)"), "-50");
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(savedProfile.format_items).toEqual([
        { format_id: "preferred", score: 100 },
        { format_id: "avoid", score: -50 },
      ]),
    );
  });

  it("warns about invalid scores from failed saves and replaces them with valid IDs", async () => {
    savedProfile.format_items = [{ format_id: "", score: 100 }];
    const user = userEvent.setup();
    show();
    await openEditor(user);
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Re-enter the affected custom-format scores",
    );
    await user.clear(screen.getByLabelText("Preferred group score (points)"));
    await user.type(
      screen.getByLabelText("Preferred group score (points)"),
      "100",
    );
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(savedProfile.format_items).toEqual([
        { format_id: "preferred", score: 100 },
      ]),
    );
  });
});
