import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Events } from "@wailsio/runtime";
import { toast } from "sonner";
import { beforeEach, describe, expect, it, vi } from "vitest";

// The mock seeds its workspace from location.search at import time.
vi.hoisted(() => window.history.replaceState({}, "", "/?view=dashboard"));

// The tray talks to the dashboard through Wails events, which only register
// inside the webview. Pretend to be inside it and capture the listeners.
vi.mock("@/lib/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api")>()),
  inWails: true,
}));
vi.mock("@wailsio/runtime", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@wailsio/runtime")>();
  return { ...actual, Events: { ...actual.Events, On: vi.fn(() => () => {}) } };
});

import { TooltipProvider } from "@/components/ui/tooltip";
import { Dashboard } from "@/components/dashboard/Dashboard";
import { api, OPEN_SETTINGS, START_NEEDS_LOGIN, UPDATE_AVAILABLE, type Workspace } from "@/lib/api";

type Handler = (e: { data: unknown }) => void;
/** The newest listener registered for an event name. */
function handler(name: string): Handler {
  const calls = vi.mocked(Events.On).mock.calls.filter((c) => c[0] === name);
  const call = calls[calls.length - 1];
  if (!call) throw new Error(`no listener for ${name}`);
  return call[1] as Handler;
}
const emit = (name: string, data: unknown = null) => act(() => handler(name)({ data }));

describe("Dashboard inside Wails", () => {
  let workspace: Workspace;
  beforeEach(async () => {
    vi.mocked(Events.On).mockClear();
    workspace = (await api.Workspace()) as unknown as Workspace;
    render(
      <TooltipProvider>
        <Dashboard workspace={workspace} />
      </TooltipProvider>,
    );
  });

  it("subscribes to the tray and updater events", () => {
    const names = vi.mocked(Events.On).mock.calls.map((c) => c[0]);
    expect(names).toEqual(expect.arrayContaining([OPEN_SETTINGS, START_NEEDS_LOGIN, UPDATE_AVAILABLE]));
  });

  it("opens settings when the tray asks", async () => {
    emit(OPEN_SETTINGS);
    expect(await screen.findByRole("dialog", { name: /settings/i })).toBeInTheDocument();
  });

  it("offers an update the background check found", async () => {
    emit(UPDATE_AVAILABLE, { available: true, version: "0.3.0", state: "available" });
    expect(await screen.findByRole("button", { name: /update to 0\.3\.0/i })).toBeInTheDocument();
  });

  it("signs in first, then starts the session the tray asked for", async () => {
    vi.spyOn(api, "StartSSOLogin").mockResolvedValue({ verificationUri: "https://device.example", userCode: "" });
    let finish!: (s: never[]) => void;
    vi.spyOn(api, "WaitSSOLogin").mockReturnValue(new Promise((res) => (finish = res)) as never);
    const start = vi.spyOn(api, "Start").mockResolvedValue({} as never);
    const success = vi.spyOn(toast, "success");
    const session = workspace.sessions.find((s) => s.name === "prod-admin")!;
    emit(START_NEEDS_LOGIN, { sessionId: session.id, integrationId: "acme-eu" });
    expect(await screen.findByRole("dialog", { name: /sign in to acme-eu/i })).toBeInTheDocument();
    finish([]);
    await waitFor(() => expect(start).toHaveBeenCalledWith(session.id, ""));
    await waitFor(() => expect(success).toHaveBeenCalledWith("Started"));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("drops the pending start when the sign-in is cancelled", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "StartSSOLogin").mockReturnValue(new Promise(() => {}) as never);
    vi.spyOn(api, "CancelSSOLogin").mockResolvedValue();
    const start = vi.spyOn(api, "Start");
    emit(START_NEEDS_LOGIN, { sessionId: workspace.sessions[0].id, integrationId: "acme-eu" });
    expect(await screen.findByRole("dialog", { name: /sign in to acme-eu/i })).toBeInTheDocument();
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(start).not.toHaveBeenCalled();
  });

  it("re-reads gcloud credentials and starts a Google Cloud session without a dialog", async () => {
    const sync = vi.spyOn(api, "SyncGCP").mockResolvedValue([]);
    const start = vi.spyOn(api, "Start").mockResolvedValue({} as never);
    const success = vi.spyOn(toast, "success");
    const session = workspace.sessions.find((s) => s.name === "data-platform")!;
    emit(START_NEEDS_LOGIN, { sessionId: session.id, integrationId: "gcp" });
    await waitFor(() => expect(success).toHaveBeenCalledWith("Started"));
    expect(sync).toHaveBeenCalledWith("gcp");
    expect(start).toHaveBeenCalledWith(session.id, "");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("ignores a start for an integration it does not know", async () => {
    const start = vi.spyOn(api, "Start");
    emit(START_NEEDS_LOGIN, { sessionId: workspace.sessions[0].id, integrationId: "ghost" });
    await act(async () => {});
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(start).not.toHaveBeenCalled();
  });
});
