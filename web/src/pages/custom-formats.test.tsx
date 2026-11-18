import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { CustomFormatsPage } from "./custom-formats";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const releaseTitle = "Example.Show.S01E01.1080p.web.h264-GROUP.mkv";
const release = {
  quality: "unknown-1080p",
  resolution: "1080p",
  codec: "h264",
  source: "WebDL",
};
const matches = [
  {
    custom_format_id: "preferred",
    custom_format_name: "Preferred group",
    score: 0,
  },
  { custom_format_id: "avoid", custom_format_name: "Avoid group", score: 0 },
];
const fetchMock = vi.fn<typeof fetch>();
const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
let failTest = false;
let failProfiles = false;

function show() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  render(
    <QueryClientProvider client={client}>
      <CustomFormatsPage />
    </QueryClientProvider>,
  );
}

async function openTester(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: "Test" }));
  await screen.findByRole("option", { name: "Normal profile" });
  await user.type(
    screen.getByRole("textbox", { name: "Release title" }),
    releaseTitle,
  );
  return within(screen.getByRole("dialog"));
}

beforeEach(() => {
  failTest = false;
  failProfiles = false;
  vi.stubGlobal("fetch", fetchMock);
  fetchMock.mockImplementation(async (input, init) => {
    const url = String(input);
    if (url.endsWith("/custom-formats/test")) {
      if (failTest) return json({ error: { message: "Test failed" } }, 500);
      const body = JSON.parse(String(init?.body));
      if (!body.quality_profile_id) return json({ release, matches, score: 0 });
      const strict = body.quality_profile_id === "strict";
      return json({
        release,
        quality_profile_id: body.quality_profile_id,
        matches: [
          { ...matches[0], score: 100 },
          { ...matches[1], score: -25 },
        ],
        score: 75,
        minimum_score: strict ? 100 : 50,
        meets_minimum: !strict,
      });
    }
    if (url.endsWith("/quality-profiles")) {
      if (failProfiles) return json({ error: { message: "Unavailable" } }, 500);
      return json({
        data: [
          { id: "normal", name: "Normal profile" },
          { id: "strict", name: "Strict profile" },
        ],
      });
    }
    if (url.endsWith("/custom-formats")) return json({ data: [] });
    throw new Error(`Unexpected request: ${url}`);
  });
});

afterEach(() => {
  vi.unstubAllGlobals();
  fetchMock.mockReset();
});

describe("Custom format tester", () => {
  it("shows matches without presenting compatibility zero scores as profile scores", async () => {
    const user = userEvent.setup();
    show();
    const dialog = await openTester(user);
    await user.click(dialog.getByRole("button", { name: "Test" }));
    expect(await dialog.findByText("Preferred group")).toBeInTheDocument();
    expect(dialog.getByText("Avoid group")).toBeInTheDocument();
    expect(
      dialog.getByText("Match-only result — no profile score calculated."),
    ).toBeInTheDocument();
    expect(dialog.queryByText(/0 points/)).not.toBeInTheDocument();
    expect(dialog.getByText("unknown-1080p")).toBeInTheDocument();
    const request = fetchMock.mock.calls.find(
      ([, init]) => init?.method === "POST",
    );
    expect(JSON.parse(String(request?.[1]?.body))).toEqual({
      title: releaseTitle,
    });
  });

  it("sends the selected profile, shows its contributions and minimum result, and clears stale results", async () => {
    const user = userEvent.setup();
    show();
    const dialog = await openTester(user);
    const selector = dialog.getByLabelText("Quality profile for scoring");
    await user.selectOptions(selector, "normal");
    await user.click(dialog.getByRole("button", { name: "Test" }));
    expect(await dialog.findByText("Meets minimum score")).toBeInTheDocument();
    expect(dialog.getByText("100 points")).toBeInTheDocument();
    expect(dialog.getByText("-25 points")).toBeInTheDocument();
    expect(
      dialog.getByText(/Total custom format score:/).parentElement,
    ).toHaveTextContent("Total custom format score: 75 points");
    expect(
      dialog.getByText(/Minimum custom format score:/).parentElement,
    ).toHaveTextContent("Minimum custom format score: 50 points");

    await user.selectOptions(selector, "strict");
    expect(dialog.queryByRole("status")).not.toBeInTheDocument();
    await user.click(dialog.getByRole("button", { name: "Test" }));
    expect(await dialog.findByText("Below minimum score")).toBeInTheDocument();
    const requests = fetchMock.mock.calls.filter(
      ([, init]) => init?.method === "POST",
    );
    expect(requests.map(([, init]) => JSON.parse(String(init?.body)))).toEqual([
      { title: releaseTitle, quality_profile_id: "normal" },
      { title: releaseTitle, quality_profile_id: "strict" },
    ]);

    await user.type(
      dialog.getByRole("textbox", { name: "Release title" }),
      ".changed",
    );
    expect(dialog.queryByRole("status")).not.toBeInTheDocument();
    await user.click(dialog.getByRole("button", { name: "Test" }));
    await dialog.findByText("Below minimum score");
    await user.selectOptions(selector, "");
    expect(dialog.queryByText("Below minimum score")).not.toBeInTheDocument();
    await user.click(dialog.getByRole("button", { name: "Test" }));
    await dialog.findByText("Match-only result — no profile score calculated.");
    expect(dialog.queryByText(/75 points/)).not.toBeInTheDocument();
  });

  it("clears a previous successful score if a retest fails", async () => {
    const user = userEvent.setup();
    show();
    const dialog = await openTester(user);
    await user.selectOptions(
      dialog.getByLabelText("Quality profile for scoring"),
      "normal",
    );
    await user.click(dialog.getByRole("button", { name: "Test" }));
    await dialog.findByText("Meets minimum score");
    failTest = true;
    await user.click(dialog.getByRole("button", { name: "Test" }));
    await waitFor(() =>
      expect(dialog.getByRole("button", { name: "Test" })).toBeEnabled(),
    );
    expect(dialog.queryByText("Meets minimum score")).not.toBeInTheDocument();
    expect(dialog.queryByText(/75 points/)).not.toBeInTheDocument();
  });

  it("keeps match-only testing available when profiles fail to load", async () => {
    failProfiles = true;
    const user = userEvent.setup();
    show();
    await user.click(screen.getByRole("button", { name: "Test" }));
    expect(
      await screen.findByText(
        "Could not load profiles. Match-only testing is still available.",
      ),
    ).toBeInTheDocument();
    const dialog = within(screen.getByRole("dialog"));
    await user.type(
      dialog.getByRole("textbox", { name: "Release title" }),
      releaseTitle,
    );
    await user.click(dialog.getByRole("button", { name: "Test" }));
    expect(await dialog.findByText("Preferred group")).toBeInTheDocument();
  });
});
