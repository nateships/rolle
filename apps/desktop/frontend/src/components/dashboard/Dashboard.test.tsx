import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

// The mock seeds its workspace from location.search at import time.
vi.hoisted(() => window.history.replaceState({}, "", "/?view=dashboard"));

import { TooltipProvider } from "@/components/ui/tooltip";
import { Dashboard } from "@/components/dashboard/Dashboard";
import { api, type Workspace } from "@/lib/api";

const rowNames = () =>
  screen
    .getAllByRole("rowgroup")
    .filter((g) => g.tagName === "TBODY")
    .flatMap((body) =>
      within(body)
        .getAllByRole("row")
        .map((r) => r.querySelector("p")?.textContent ?? ""),
    );

describe("Dashboard", () => {
  let workspace: Workspace;
  beforeEach(async () => {
    workspace = (await api.Workspace()) as unknown as Workspace;
    window.history.replaceState({}, "", "/?view=dashboard");
  });

  function renderDashboard() {
    return render(
      <TooltipProvider>
        <Dashboard workspace={workspace} />
      </TooltipProvider>,
    );
  }

  it("renders the seeded mock sessions and sidebar", () => {
    renderDashboard();
    expect(workspace.sessions).toHaveLength(8);
    expect(screen.getByText("All sessions", { selector: "span" })).toBeInTheDocument();
    for (const alias of ["acme", "contoso", "gcp", "acme-eu"]) expect(screen.getByText(alias)).toBeInTheDocument();
    // Account groups and standalone rows from the seed. Acme Prod also heads the favorites panel.
    expect(screen.getAllByText("Acme Prod")).toHaveLength(2);
    expect(screen.getByText("Acme Dev")).toBeInTheDocument();
    expect(screen.getByText("Contoso Production")).toBeInTheDocument();
    expect(screen.getByText("data-platform")).toBeInTheDocument();
    // Favorites appear in their own panel above the full list.
    expect(screen.getByRole("heading", { name: /favorites/i })).toBeInTheDocument();
    expect(screen.getAllByText("deployer")).toHaveLength(2);
    expect(screen.getByText("2 active")).toBeInTheDocument();
  });

  it("filters to active sessions", async () => {
    const user = userEvent.setup();
    renderDashboard();
    await user.click(screen.getByRole("button", { name: /^Active/ }));
    expect(rowNames()).toEqual(["Acme Prod", "AdministratorAccess", "Contoso Production"]);
    expect(screen.queryByRole("heading", { name: /favorites/i })).not.toBeInTheDocument();
  });

  it("filters to favorites", async () => {
    const user = userEvent.setup();
    renderDashboard();
    await user.click(screen.getByRole("button", { name: /^Favorites/ }));
    expect(rowNames()).toEqual(["Acme Prod", "AdministratorAccess", "deployer"]);
  });

  it("filters to one integration", async () => {
    const user = userEvent.setup();
    renderDashboard();
    await user.click(screen.getByRole("button", { name: "contoso" }));
    expect(rowNames()).toEqual(["Contoso Production"]);
    expect(screen.queryByText("Acme Prod")).not.toBeInTheDocument();
  });

  it("tells the user when a filtered integration is signed out", async () => {
    const user = userEvent.setup();
    renderDashboard();
    await user.click(screen.getByRole("button", { name: "acme-eu" }));
    expect(screen.getByText("acme-eu is signed out")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /sign in to acme-eu/i })).toBeInTheDocument();
  });

  it("narrows rows with the search box", async () => {
    const user = userEvent.setup();
    renderDashboard();
    await user.type(screen.getByPlaceholderText("Search sessions"), "deployer");
    expect(rowNames()).toEqual(["deployer", "deployer"]);
    await user.clear(screen.getByPlaceholderText("Search sessions"));
    await user.type(screen.getByPlaceholderText("Search sessions"), "readonly");
    // A search keeps the matching account group visible with only the matching role.
    expect(rowNames()).toEqual(["Acme Prod", "ReadOnlyAccess"]);
    await user.clear(screen.getByPlaceholderText("Search sessions"));
    await user.type(screen.getByPlaceholderText("Search sessions"), "zzz");
    expect(screen.getByText("Nothing matches")).toBeInTheDocument();
  });

  it("reads the initial filter from the query string", async () => {
    window.history.replaceState({}, "", "/?view=dashboard&filter=favorites");
    renderDashboard();
    expect(rowNames()).toEqual(["Acme Prod", "AdministratorAccess", "deployer"]);
  });

  it("opens settings and the shortcut sheet from the keyboard", async () => {
    const user = userEvent.setup();
    renderDashboard();
    await user.keyboard("{Meta>},{/Meta}");
    expect(await screen.findByRole("dialog", { name: /settings/i })).toBeInTheDocument();
    // The same key closes it again.
    await user.keyboard("{Meta>},{/Meta}");
    await waitFor(() => expect(screen.queryByRole("dialog", { name: /settings/i })).not.toBeInTheDocument());
    await user.keyboard("{Control>}/{/Control}");
    expect(await screen.findByRole("dialog", { name: /keyboard shortcuts/i })).toBeInTheDocument();
  });

  it("offers a found update in the sidebar footer", async () => {
    window.history.replaceState({}, "", "/?view=dashboard&update=1");
    renderDashboard();
    expect(await screen.findByRole("button", { name: /update to 0\.2\.0/i }, { timeout: 3000 })).toBeInTheDocument();
  });

  it("shows shortcut badges while the modifier is held", async () => {
    const user = userEvent.setup();
    renderDashboard();
    await user.keyboard("{Control>}");
    expect(await screen.findByText("Ctrl+1")).toBeInTheDocument();
    expect(screen.getByText("Ctrl+F")).toBeInTheDocument();
    await user.keyboard("{/Control}");
    await waitFor(() => expect(screen.queryByText("Ctrl+1")).not.toBeInTheDocument());
  });
});
