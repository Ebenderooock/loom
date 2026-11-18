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
    { custom_format_id: "preferred", score: 100 },
    { format_id: "avoid", score: -50 },
    { custom_format_id: "neutral", score: 0 },
    { custom_format_id: "unavailable-format", score: 17 },
  ],
};
let savedProfile = structuredClone(initialProfile);
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
  vi.stubGlobal("fetch", fetchMock);
  fetchMock.mockImplementation(async (input, init) => {
    const url = String(input);
    if (url.endsWith("/movies/quality-definitions")) return json(definitions);
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

  it("creates a profile with its selected qualities, format score and minimum", async () => {
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
      format_items: [{ custom_format_id: "preferred", score: 100 }],
    });
    expect(JSON.parse(body.items)).toEqual([
      expect.objectContaining({ id: "web-1080p", allowed: true }),
      expect.objectContaining({ id: "bluray-1080p", allowed: false }),
    ]);
  });
});
