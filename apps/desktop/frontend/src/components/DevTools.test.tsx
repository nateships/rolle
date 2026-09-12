import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { DevTools } from "@/components/DevTools";
import { api } from "@/lib/api";

/** Render and let the mount-time mode lookups settle. */
async function renderDev() {
  let view!: ReturnType<typeof render>;
  await act(async () => {
    view = render(<DevTools />);
  });
  return view;
}

describe("DevTools", () => {
  beforeEach(() => {
    window.history.replaceState({}, "", "/");
    vi.spyOn(api, "DevMode").mockResolvedValue(true);
    vi.spyOn(api, "DemoMode").mockResolvedValue(false);
  });

  it("renders nothing in a production build", async () => {
    vi.spyOn(api, "DevMode").mockResolvedValue(false);
    const { container } = await renderDev();
    expect(container).toBeEmptyDOMElement();
  });

  it("hides for screenshots", async () => {
    window.history.replaceState({}, "", "/?shot=1");
    const { container } = await renderDev();
    expect(container).toBeEmptyDOMElement();
  });

  it("relaunches on demo data from a dev build", async () => {
    const user = userEvent.setup();
    const relaunch = vi.spyOn(api, "Relaunch").mockResolvedValue();
    const success = vi.spyOn(toast, "success");
    await renderDev();

    await user.click(await screen.findByRole("button", { name: "dev" }));
    expect(await screen.findByText("Development build")).toBeInTheDocument();
    await user.click(screen.getByRole("menuitem", { name: /relaunch with demo data/i }));

    await waitFor(() => expect(relaunch).toHaveBeenCalledWith(true));
    expect(success).toHaveBeenCalledWith("Relaunching on demo data");
  });

  it("relaunches on real data from a demo build", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "DemoMode").mockResolvedValue(true);
    const relaunch = vi.spyOn(api, "Relaunch").mockResolvedValue();
    const success = vi.spyOn(toast, "success");
    await renderDev();

    await user.click(await screen.findByRole("button", { name: "demo" }));
    expect(await screen.findByText("Development build · fictional data")).toBeInTheDocument();
    await user.click(screen.getByRole("menuitem", { name: /relaunch with real data/i }));

    await waitFor(() => expect(relaunch).toHaveBeenCalledWith(false));
    expect(success).toHaveBeenCalledWith("Relaunching on your real data");
  });

  it("replays onboarding", async () => {
    const user = userEvent.setup();
    const replay = vi.spyOn(api, "ReplayOnboarding").mockResolvedValue();
    const success = vi.spyOn(toast, "success");
    await renderDev();

    await user.click(await screen.findByRole("button", { name: "dev" }));
    await user.click(await screen.findByRole("menuitem", { name: /replay onboarding/i }));

    await waitFor(() => expect(success).toHaveBeenCalledWith("Onboarding will replay"));
    expect(replay).toHaveBeenCalledTimes(1);
  });

  it("resets the workspace only on the second click", async () => {
    const user = userEvent.setup();
    const reset = vi.spyOn(api, "Reset").mockResolvedValue();
    const success = vi.spyOn(toast, "success");
    await renderDev();

    await user.click(await screen.findByRole("button", { name: "dev" }));
    await user.click(await screen.findByRole("menuitem", { name: "Reset workspace" }));
    // The menu stays open and asks for confirmation.
    const confirm = await screen.findByRole("menuitem", { name: "Click again to reset" });
    expect(reset).not.toHaveBeenCalled();
    await user.click(confirm);

    await waitFor(() => expect(reset).toHaveBeenCalledTimes(1));
    expect(success).toHaveBeenCalledWith("Workspace reset");
  });

  it("disarms the reset when the menu closes", async () => {
    const user = userEvent.setup();
    await renderDev();

    await user.click(await screen.findByRole("button", { name: "dev" }));
    await user.click(await screen.findByRole("menuitem", { name: "Reset workspace" }));
    await screen.findByRole("menuitem", { name: "Click again to reset" });
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("menu")).not.toBeInTheDocument());

    await user.click(screen.getByRole("button", { name: "dev" }));
    expect(await screen.findByRole("menuitem", { name: "Reset workspace" })).toBeInTheDocument();
  });

  it("shows an action failure", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "Relaunch").mockRejectedValue(new Error("no binary"));
    const error = vi.spyOn(toast, "error");
    await renderDev();

    await user.click(await screen.findByRole("button", { name: "dev" }));
    await user.click(await screen.findByRole("menuitem", { name: /relaunch with demo data/i }));
    await waitFor(() => expect(error).toHaveBeenCalledWith("no binary"));
  });
});
