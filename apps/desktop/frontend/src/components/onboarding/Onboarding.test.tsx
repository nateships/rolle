import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@/components/ui/tooltip";
import { Onboarding } from "@/components/onboarding/Onboarding";
import { api, type Workspace } from "@/lib/api";

const empty = { version: 1, onboarded: false, sessions: [], integrations: [] } as unknown as Workspace;

describe("Onboarding", () => {
  beforeEach(() => {
    window.history.replaceState({}, "", "/");
    // The cloud step scans the machine; keep the test offline and instant.
    vi.spyOn(api, "Discover").mockResolvedValue({ awsPortals: [], azureTenants: [], gcp: null } as never);
  });

  it("renders the welcome step", () => {
    render(
      <TooltipProvider>
        <Onboarding workspace={empty} />
      </TooltipProvider>,
    );
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent("Assume any role, any cloud.");
    expect(screen.getByRole("button", { name: /get started/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Skip" })).toBeEnabled();
  });

  it("advances to the cloud step on the primary button", async () => {
    const user = userEvent.setup();
    render(
      <TooltipProvider>
        <Onboarding workspace={empty} />
      </TooltipProvider>,
    );
    await user.click(screen.getByRole("button", { name: /get started/i }));
    expect(await screen.findByRole("heading", { level: 2 })).toHaveTextContent("Where do your roles live?");
    expect(screen.queryByRole("button", { name: /get started/i })).not.toBeInTheDocument();
    expect(history.state).toMatchObject({ step: "cloud" });
  });

  it("jumps to a step named in the query string", () => {
    window.history.replaceState({}, "", "/?step=cloud");
    render(
      <TooltipProvider>
        <Onboarding workspace={empty} />
      </TooltipProvider>,
    );
    expect(screen.getByRole("heading", { level: 2 })).toHaveTextContent("Where do your roles live?");
  });

  it("offers the rolle command on the final step", async () => {
    window.history.replaceState({}, "", "/?step=done");
    render(
      <TooltipProvider>
        <Onboarding workspace={empty} />
      </TooltipProvider>,
    );
    expect(await screen.findByRole("button", { name: /install command/i })).toBeInTheDocument();
  });

  it("skips to the final step without a cloud", async () => {
    const user = userEvent.setup();
    render(
      <TooltipProvider>
        <Onboarding workspace={empty} />
      </TooltipProvider>,
    );
    await user.click(screen.getByRole("button", { name: "Skip" }));
    expect(await screen.findByRole("heading", { level: 2 })).toHaveTextContent("You're all set.");
    expect(await screen.findByRole("button", { name: /install command/i })).toBeInTheDocument();
  });
});
