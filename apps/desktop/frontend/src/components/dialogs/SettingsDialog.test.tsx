import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { beforeEach, describe, expect, it, vi } from "vitest";

// The terminal list depends on the platform at import time. Pretend to be a Mac so it has options.
vi.hoisted(() =>
  Object.defineProperty(window.navigator, "userAgent", { value: "Mozilla/5.0 (Macintosh)", configurable: true }),
);

import { SettingsDialog } from "@/components/dialogs/SettingsDialog";
import { api, type Settings } from "@/lib/api";

const base: Settings = {
  theme: "system",
  defaultRegion: "us-east-1",
  assumeRoleMinutes: 60,
  hideOnClose: true,
  notifyOff: false,
  verboseLogging: false,
  autoUpdateOff: false,
  updateChannel: "",
  terminal: "",
  proxyUrl: "",
  caBundle: "",
} as unknown as Settings;

/** The region picker: a combobox that shows the selected region code. */
const regionBox = () => screen.getAllByRole("combobox").find((c) => c.textContent?.startsWith("us-east-1"))!;

/** Open a Radix select and choose one option by its label. */
async function pick(user: ReturnType<typeof userEvent.setup>, trigger: HTMLElement, option: string) {
  await user.click(trigger);
  await user.click(await screen.findByRole("option", { name: option }));
}

describe("SettingsDialog", () => {
  let update: ReturnType<typeof vi.spyOn>;
  beforeEach(() => {
    window.history.replaceState({}, "", "/");
    vi.spyOn(api, "Settings").mockResolvedValue({ ...base });
    update = vi.spyOn(api, "UpdateSettings").mockImplementation(((s: Settings) => Promise.resolve({ ...s })) as never);
  });

  /** Render and let the settings and app info loads settle. */
  async function open(onClose = vi.fn()) {
    await act(async () => {
      render(<SettingsDialog open onClose={onClose} />);
    });
    await screen.findByRole("tab", { name: "General" });
    return onClose;
  }

  async function tab(user: ReturnType<typeof userEvent.setup>, name: string) {
    await user.click(screen.getByRole("tab", { name }));
  }

  it("writes each general switch through UpdateSettings and confirms the save", async () => {
    const user = userEvent.setup();
    await open();

    await user.click(screen.getByRole("switch", { name: "Keep running in the tray" }));
    await waitFor(() => expect(update).toHaveBeenCalledWith(expect.objectContaining({ hideOnClose: false })));
    expect(await screen.findByText("Saved")).toBeInTheDocument();

    // The lead time shows while notifications are on and defaults to two minutes.
    await user.click(screen.getByRole("combobox", { name: "Warn before a session expires" }));
    await user.click(await screen.findByRole("option", { name: "5 minutes" }));
    await waitFor(() => expect(update).toHaveBeenCalledWith(expect.objectContaining({ notifyLeadMinutes: 5 })));

    await user.click(screen.getByRole("switch", { name: "Expiry notifications" }));
    await waitFor(() => expect(update).toHaveBeenCalledWith(expect.objectContaining({ notifyOff: true })));

    await user.click(screen.getByRole("switch", { name: "Automatic updates" }));
    await waitFor(() => expect(update).toHaveBeenCalledWith(expect.objectContaining({ autoUpdateOff: true })));

    await tab(user, "Advanced");
    await user.click(screen.getByRole("switch", { name: "Verbose logging" }));
    await waitFor(() => expect(update).toHaveBeenCalledWith(expect.objectContaining({ verboseLogging: true })));
  });

  it("hides and shows sidebar sections from Appearance", async () => {
    const user = userEvent.setup();
    await open();
    await tab(user, "Appearance");
    await user.click(screen.getByRole("switch", { name: "Azure tenants" }));
    await waitFor(() => expect(update).toHaveBeenCalledWith(expect.objectContaining({ hiddenSections: ["azure"] })));
  });

  it("writes the duration, terminal, and default region", async () => {
    const user = userEvent.setup();
    await open();

    await pick(user, screen.getByRole("combobox", { name: "Terminal app" }), "Ghostty");
    await waitFor(() => expect(update).toHaveBeenCalledWith(expect.objectContaining({ terminal: "ghostty" })));

    await tab(user, "AWS");
    await pick(user, screen.getByRole("combobox", { name: "Assume role duration" }), "2 hours");
    await waitFor(() => expect(update).toHaveBeenCalledWith(expect.objectContaining({ assumeRoleMinutes: 120 })));

    await user.click(regionBox());
    await user.click(await screen.findByText("eu-west-1"));
    await waitFor(() => expect(update).toHaveBeenCalledWith(expect.objectContaining({ defaultRegion: "eu-west-1" })));
  });

  it("reverts a switch when the save fails", async () => {
    const user = userEvent.setup();
    update.mockRejectedValue(new Error("disk full"));
    const error = vi.spyOn(toast, "error");
    await open();

    const tray = screen.getByRole("switch", { name: "Keep running in the tray" });
    expect(tray).toBeChecked();
    await user.click(tray);
    await waitFor(() => expect(error).toHaveBeenCalledWith("disk full"));
    expect(screen.getByRole("switch", { name: "Keep running in the tray" })).toBeChecked();
  });

  it("saves the login item from the general tab", async () => {
    const user = userEvent.setup();
    await open();
    const item = screen.getByRole("switch", { name: "Start at login" });
    expect(item).not.toBeChecked();
    await user.click(item);
    await waitFor(() => expect(update).toHaveBeenCalledWith(expect.objectContaining({ loginItem: true })));
    expect(screen.getByRole("switch", { name: "Start at login" })).toBeChecked();
  });

  it("stores the dock switch inverted, shown by default", async () => {
    const user = userEvent.setup();
    await open();
    await tab(user, "Appearance");
    const dock = screen.getByRole("switch", { name: "Dock icon" });
    expect(dock).toBeChecked();
    await user.click(dock);
    await waitFor(() => expect(update).toHaveBeenCalledWith(expect.objectContaining({ hideDock: true })));
    expect(screen.getByRole("switch", { name: "Dock icon" })).not.toBeChecked();
  });

  it("stores the dock badge switch inverted, shown by default", async () => {
    const user = userEvent.setup();
    await open();
    await tab(user, "Appearance");
    const badge = screen.getByRole("switch", { name: "Dock badge" });
    expect(badge).toBeChecked();
    await user.click(badge);
    await waitFor(() => expect(update).toHaveBeenCalledWith(expect.objectContaining({ dockBadgeOff: true })));
  });

  it("applies and saves the theme", async () => {
    const user = userEvent.setup();
    await open();
    await tab(user, "Appearance");

    await user.click(screen.getByRole("button", { name: "Dark" }));
    await waitFor(() => expect(update).toHaveBeenCalledWith(expect.objectContaining({ theme: "dark" })));
    expect(document.documentElement).toHaveClass("dark");
    await user.click(screen.getByRole("button", { name: "Light" }));
    await waitFor(() => expect(update).toHaveBeenCalledWith(expect.objectContaining({ theme: "light" })));
    expect(document.documentElement).not.toHaveClass("dark");
  });

  it("checks for updates and offers to install the one it finds", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "CheckForUpdates").mockResolvedValue({
      enabled: true,
      currentVersion: "0.0.1",
      available: true,
      version: "0.2.0",
      state: "available",
    });
    const install = vi.spyOn(api, "InstallUpdate").mockResolvedValue();
    const success = vi.spyOn(toast, "success");
    await open();
    await tab(user, "About");

    expect(await screen.findByText("rolle 0.0.1-dev")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /check for updates/i }));

    expect(await screen.findByText("Version 0.2.0 is ready to install.")).toBeInTheDocument();
    expect(success).toHaveBeenCalledWith("rolle 0.2.0 is available");
    await user.click(screen.getByRole("button", { name: /install 0\.2\.0/i }));
    await waitFor(() => expect(install).toHaveBeenCalledTimes(1));
  });

  it("says when the app is current and when updates are disabled", async () => {
    const user = userEvent.setup();
    const check = vi.spyOn(api, "CheckForUpdates").mockResolvedValue({
      enabled: true,
      currentVersion: "0.0.1",
      available: false,
      state: "up-to-date",
    });
    const success = vi.spyOn(toast, "success");
    const info = vi.spyOn(toast, "info");
    await open();
    await tab(user, "About");

    await user.click(screen.getByRole("button", { name: /check for updates/i }));
    expect(await screen.findByText("You're on the latest version.")).toBeInTheDocument();
    expect(success).toHaveBeenCalledWith("You're on the latest version");

    check.mockResolvedValue({ enabled: false, currentVersion: "0.0.1", available: false, state: "disabled" });
    await user.click(screen.getByRole("button", { name: /check for updates/i }));
    expect(await screen.findByText("Development build, updates disabled.")).toBeInTheDocument();
    expect(info).toHaveBeenCalledWith("Updates are disabled in development builds");
  });

  it("shows an update check failure", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "CheckForUpdates").mockRejectedValue(new Error("offline"));
    const error = vi.spyOn(toast, "error");
    await open();
    await tab(user, "About");

    await user.click(screen.getByRole("button", { name: /check for updates/i }));
    await waitFor(() => expect(error).toHaveBeenCalledWith("offline"));
  });

  it("switches the update channel and copies a file path", async () => {
    const user = userEvent.setup();
    const success = vi.spyOn(toast, "success");
    await open();
    await tab(user, "About");

    await pick(user, screen.getByRole("combobox", { name: "Update channel" }), "Beta");
    await waitFor(() => expect(update).toHaveBeenCalledWith(expect.objectContaining({ updateChannel: "beta" })));
    await pick(user, screen.getByRole("combobox", { name: "Update channel" }), "Stable");
    await waitFor(() => expect(update).toHaveBeenCalledWith(expect.objectContaining({ updateChannel: "" })));

    expect(await screen.findByText("~/.config/rolle/workspace.json")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Copy Workspace" }));
    await waitFor(() => expect(success).toHaveBeenCalledWith("Workspace copied"));
    expect(await navigator.clipboard.readText()).toBe("~/.config/rolle/workspace.json");
  });

  it("saves the proxy and CA bundle on blur, only when changed", async () => {
    const user = userEvent.setup();
    await open();
    await tab(user, "Advanced");

    const proxy = screen.getByPlaceholderText("http://host:port");
    await user.click(proxy);
    await user.tab();
    expect(update).not.toHaveBeenCalled();

    await user.type(proxy, "http://proxy.corp:3128 ");
    await user.tab();
    await waitFor(() =>
      expect(update).toHaveBeenCalledWith(expect.objectContaining({ proxyUrl: "http://proxy.corp:3128" })),
    );

    await user.type(screen.getByPlaceholderText("/path/to/corp-root.pem"), "/etc/root.pem");
    await user.tab();
    await waitFor(() => expect(update).toHaveBeenCalledWith(expect.objectContaining({ caBundle: "/etc/root.pem" })));
  });

  it("replays onboarding and closes", async () => {
    const user = userEvent.setup();
    const replay = vi.spyOn(api, "ReplayOnboarding").mockResolvedValue();
    const success = vi.spyOn(toast, "success");
    const onClose = await open();
    await tab(user, "Advanced");

    await user.click(screen.getByRole("button", { name: /replay onboarding/i }));
    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1));
    expect(replay).toHaveBeenCalledTimes(1);
    expect(success).toHaveBeenCalledWith("Onboarding will replay");
  });

  it("asks twice before a reset", async () => {
    const user = userEvent.setup();
    const reset = vi.spyOn(api, "Reset").mockResolvedValue();
    const success = vi.spyOn(toast, "success");
    const onClose = await open();
    await tab(user, "Advanced");

    await user.click(screen.getByRole("button", { name: /reset rolle/i }));
    expect(reset).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: /yes, remove everything/i }));
    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1));
    expect(reset).toHaveBeenCalledTimes(1);
    expect(success).toHaveBeenCalledWith("rolle was reset");
  });

  it("shows a reset failure and stays open", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "Reset").mockRejectedValue(new Error("keychain locked"));
    const error = vi.spyOn(toast, "error");
    const onClose = await open();
    await tab(user, "Advanced");

    await user.click(screen.getByRole("button", { name: /reset rolle/i }));
    await user.click(screen.getByRole("button", { name: /yes, remove everything/i }));
    await waitFor(() => expect(error).toHaveBeenCalledWith("keychain locked"));
    expect(onClose).not.toHaveBeenCalled();
  });

  it("opens the tab named in the query string", async () => {
    window.history.replaceState({}, "", "/?tab=about");
    await open();
    expect(screen.getByRole("tab", { name: "About" })).toHaveAttribute("aria-selected", "true");
  });

  it("reports a settings load failure", async () => {
    vi.spyOn(api, "Settings").mockRejectedValue(new Error("corrupt file"));
    const error = vi.spyOn(toast, "error");
    await act(async () => {
      render(<SettingsDialog open onClose={vi.fn()} />);
    });
    await waitFor(() => expect(error).toHaveBeenCalledWith("corrupt file"));
    expect(screen.queryByRole("tab")).not.toBeInTheDocument();
  });

  it("shows every section switch on and turns a hidden one back on", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "Settings").mockResolvedValue({ ...base, hiddenSections: ["azure", "gcp"] } as Settings);
    await open();
    await tab(user, "Appearance");
    const azure = screen.getByRole("switch", { name: "Azure tenants" });
    expect(azure).not.toBeChecked();
    expect(screen.getByRole("switch", { name: "AWS Identity Center" })).toBeChecked();
    await user.click(azure);
    // Only that key leaves the list; the other hidden section stays hidden.
    await waitFor(() => expect(update).toHaveBeenCalledWith(expect.objectContaining({ hiddenSections: ["gcp"] })));
    expect(screen.getByRole("switch", { name: "Azure tenants" })).toBeChecked();
  });

  it("keeps a toggle made while an earlier save is still running", async () => {
    const user = userEvent.setup();
    let finish!: (s: Settings) => void;
    update.mockImplementationOnce(((s: Settings) => new Promise((res) => (finish = (v) => res(v ?? s)))) as never);
    await open();

    await user.click(screen.getByRole("switch", { name: "Keep running in the tray" }));
    await user.click(screen.getByRole("switch", { name: "Expiry notifications" }));
    await waitFor(() => expect(update).toHaveBeenCalledTimes(2));
    // The first save answers late and without the second change; that change stays.
    act(() => finish({ ...base, hideOnClose: false }));
    await waitFor(() => expect(screen.getByText("Saved")).toBeInTheDocument());
    expect(screen.getByRole("switch", { name: "Keep running in the tray" })).not.toBeChecked();
    expect(screen.getByRole("switch", { name: "Expiry notifications" })).not.toBeChecked();
  });

  it("reverts the theme when its save fails", async () => {
    const user = userEvent.setup();
    update.mockRejectedValue(new Error("read-only"));
    const error = vi.spyOn(toast, "error");
    await open();
    await tab(user, "Appearance");
    await user.click(screen.getByRole("button", { name: "Dark" }));
    await waitFor(() => expect(error).toHaveBeenCalledWith("read-only"));
    expect(document.documentElement).not.toHaveClass("dark");
  });

  it("reports a failed install", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "Info").mockRejectedValue(new Error("no info"));
    vi.spyOn(api, "CheckForUpdates").mockResolvedValue({
      enabled: true,
      currentVersion: "0.1.0",
      available: true,
      version: "0.2.0",
      notes: "",
      state: "idle",
    });
    const install = vi.spyOn(api, "InstallUpdate").mockRejectedValueOnce(new Error("cancelled"));
    const error = vi.spyOn(toast, "error");
    await open();
    await tab(user, "About");

    // A failed app info read does not keep the dialog from opening.
    expect(screen.getByRole("tab", { name: "About" })).toHaveAttribute("aria-selected", "true");
    await user.click(screen.getByRole("button", { name: /check for updates/i }));
    const installButton = await screen.findByRole("button", { name: /install/i });
    // A declined prompt is quiet; any other failure is shown.
    await user.click(installButton);
    await waitFor(() => expect(install).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(screen.getByRole("button", { name: /install/i })).toBeEnabled());
    expect(error).not.toHaveBeenCalled();
    install.mockRejectedValueOnce(new Error("bad signature"));
    await user.click(screen.getByRole("button", { name: /install/i }));
    await waitFor(() => expect(error).toHaveBeenCalledWith("bad signature"));
  });
});
