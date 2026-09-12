import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
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

  it("keeps hidden sessions out of the lists until asked", async () => {
    const user = userEvent.setup();
    const personal = workspace.sessions.find((s) => s.name === "personal")!;
    Object.assign(personal, { hidden: true });
    renderDashboard();
    expect(rowNames()).not.toContain("personal");
    expect(screen.getByText("All sessions", { selector: "span" }).closest("button")).toHaveTextContent("7");

    await user.click(screen.getByRole("button", { name: /^Hidden/ }));
    expect(rowNames()).toEqual(["personal"]);
    expect(screen.getByLabelText("Hidden")).toBeInTheDocument();

    // A search names it even on the full list.
    await user.click(screen.getByRole("button", { name: /^All sessions/ }));
    await user.type(screen.getByPlaceholderText(/search/i), "personal");
    expect(rowNames()).toContain("personal");
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
    // A shortcut while the badges show keeps them up until the modifier lifts.
    await user.keyboard("3");
    expect(screen.getByText("Ctrl+1")).toBeInTheDocument();
    await user.keyboard("{/Control}");
    await waitFor(() => expect(screen.queryByText("Ctrl+1")).not.toBeInTheDocument());
  });

  it("badges the footer buttons and the active filter while the modifier is held", async () => {
    const user = userEvent.setup();
    renderDashboard();
    await user.keyboard("{Control>}");
    expect(await screen.findByText("Ctrl+2")).toBeInTheDocument();
    expect(screen.getByText("Ctrl+3")).toBeInTheDocument();
    expect(screen.getByText("Ctrl+/")).toBeInTheDocument();
    expect(screen.getByText("Ctrl+,")).toBeInTheDocument();
    await user.keyboard("{/Control}");
  });

  it("switches filters and toggles the import dialog from the keyboard", async () => {
    const user = userEvent.setup();
    renderDashboard();
    await user.keyboard("{Control>}2{/Control}");
    expect(rowNames()).toEqual(["Acme Prod", "AdministratorAccess", "Contoso Production"]);
    await user.keyboard("{Control>}3{/Control}");
    expect(rowNames()).toEqual(["Acme Prod", "AdministratorAccess", "deployer"]);
    await user.keyboard("{Control>}1{/Control}");
    expect(rowNames()).toContain("Contoso Production");
    await user.keyboard("{Control>}i{/Control}");
    expect(await screen.findByRole("dialog", { name: /import from this machine/i })).toBeInTheDocument();
    await user.keyboard("{Control>}i{/Control}");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("clears the search with Escape", async () => {
    const user = userEvent.setup();
    renderDashboard();
    const search = screen.getByPlaceholderText("Search sessions");
    await user.type(search, "zzz");
    expect(screen.getByText("Nothing matches")).toBeInTheDocument();
    await user.keyboard("{Escape}");
    expect(search).toHaveValue("");
    expect(rowNames()).toContain("Contoso Production");
  });

  it("falls back to all sessions when the query names an unknown filter", () => {
    window.history.replaceState({}, "", "/?view=dashboard&filter=ghost");
    renderDashboard();
    expect(rowNames()).toContain("Acme Prod");
    expect(rowNames()).toContain("Contoso Production");
    expect(screen.getByRole("heading", { name: /favorites/i })).toBeInTheDocument();
  });

  it("installs a found update from the footer", async () => {
    const user = userEvent.setup();
    window.history.replaceState({}, "", "/?view=dashboard&update=1");
    const install = vi.spyOn(api, "InstallUpdate").mockResolvedValue();
    renderDashboard();
    await user.click(await screen.findByRole("button", { name: /update to 0\.2\.0/i }, { timeout: 3000 }));
    expect(install).toHaveBeenCalledTimes(1);
  });

  it("opens settings and the shortcut sheet from the footer buttons", async () => {
    const user = userEvent.setup();
    renderDashboard();
    await user.click(screen.getByRole("button", { name: "Settings" }));
    expect(await screen.findByRole("dialog", { name: /settings/i })).toBeInTheDocument();
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    await user.click(screen.getByRole("button", { name: "Keyboard shortcuts" }));
    expect(await screen.findByRole("dialog", { name: /keyboard shortcuts/i })).toBeInTheDocument();
  });

  describe("integration menus", () => {
    const openOptions = async (user: ReturnType<typeof userEvent.setup>, alias: string) => {
      await user.click(screen.getByRole("button", { name: `${alias} options` }));
      return screen.findByRole("menu");
    };
    const pick = async (user: ReturnType<typeof userEvent.setup>, alias: string, item: RegExp) => {
      const menu = await openOptions(user, alias);
      await user.click(within(menu).getByRole("menuitem", { name: item }));
    };

    it("lists the actions for a signed-in AWS portal", async () => {
      const user = userEvent.setup();
      renderDashboard();
      const menu = await openOptions(user, "acme");
      expect(
        within(menu)
          .getAllByRole("menuitem")
          .map((m) => m.textContent?.trim()),
      ).toEqual(["Rename", "Sync", "Sign out", "Remove"]);
    });

    it("syncs, signs out, and removes an AWS portal", async () => {
      const user = userEvent.setup();
      const sync = vi.spyOn(api, "SyncSSO").mockResolvedValue([]);
      const logout = vi.spyOn(api, "SSOLogout").mockResolvedValue();
      const remove = vi.spyOn(api, "RemoveIntegration").mockResolvedValue();
      const success = vi.spyOn(toast, "success");
      renderDashboard();
      await pick(user, "acme", /^sync$/i);
      await waitFor(() => expect(success).toHaveBeenCalledWith("Synced"));
      expect(sync).toHaveBeenCalledWith("acme");
      await pick(user, "acme", /sign out/i);
      await waitFor(() => expect(success).toHaveBeenCalledWith("Signed out"));
      expect(logout).toHaveBeenCalledWith("acme");
      await pick(user, "acme", /remove/i);
      await waitFor(() => expect(success).toHaveBeenCalledWith("Removed"));
      expect(remove).toHaveBeenCalledWith("acme");
    });

    it("reports a failed action", async () => {
      const user = userEvent.setup();
      vi.spyOn(api, "RemoveIntegration").mockRejectedValue(new Error("still has active sessions"));
      const error = vi.spyOn(toast, "error");
      renderDashboard();
      await pick(user, "acme", /remove/i);
      await waitFor(() => expect(error).toHaveBeenCalledWith("still has active sessions"));
    });

    it("uses the Azure calls for a tenant", async () => {
      const user = userEvent.setup();
      const sync = vi.spyOn(api, "SyncAzure").mockResolvedValue([]);
      const logout = vi.spyOn(api, "AzureLogout").mockResolvedValue();
      renderDashboard();
      await pick(user, "contoso", /^sync$/i);
      await waitFor(() => expect(sync).toHaveBeenCalledWith("contoso"));
      await pick(user, "contoso", /sign out/i);
      await waitFor(() => expect(logout).toHaveBeenCalledWith("contoso"));
    });

    it("offers sync and impersonation, but no sign-out, for Google Cloud", async () => {
      const user = userEvent.setup();
      const sync = vi.spyOn(api, "SyncGCP").mockResolvedValue([]);
      renderDashboard();
      const menu = await openOptions(user, "gcp");
      expect(within(menu).queryByRole("menuitem", { name: /sign out/i })).not.toBeInTheDocument();
      await user.click(within(menu).getByRole("menuitem", { name: /^sync$/i }));
      await waitFor(() => expect(sync).toHaveBeenCalledWith("gcp"));
      await pick(user, "gcp", /impersonate service account/i);
      expect(await screen.findByRole("dialog", { name: /impersonate a service account/i })).toBeInTheDocument();
    });

    it("re-reads gcloud credentials when a signed-out Google account signs in", async () => {
      const user = userEvent.setup();
      workspace = {
        ...workspace,
        integrations: workspace.integrations.map((i) => (i.id === "gcp" ? { ...i, gcp: { account: "" } } : i)),
      } as Workspace;
      const sync = vi.spyOn(api, "SyncGCP").mockResolvedValue([]);
      const success = vi.spyOn(toast, "success");
      renderDashboard();
      await pick(user, "gcp", /sign in/i);
      await waitFor(() => expect(success).toHaveBeenCalledWith("Synced"));
      expect(sync).toHaveBeenCalledWith("gcp");
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });

    it("opens the sign-in dialog for a signed-out portal and selects it when done", async () => {
      const user = userEvent.setup();
      vi.spyOn(api, "StartSSOLogin").mockResolvedValue({ verificationUri: "https://device.example", userCode: "" });
      let finish!: (s: never[]) => void;
      vi.spyOn(api, "WaitSSOLogin").mockReturnValue(new Promise((res) => (finish = res)) as never);
      const success = vi.spyOn(toast, "success");
      renderDashboard();
      await pick(user, "acme-eu", /sign in/i);
      expect(await screen.findByRole("dialog", { name: /sign in to acme-eu/i })).toBeInTheDocument();
      expect(api.StartSSOLogin).toHaveBeenCalledWith("acme-eu");
      finish([]);
      await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
      expect(success).toHaveBeenCalledWith("Signed in to acme-eu", expect.anything());
      // The finished sign-in filters to that portal, which has no sessions yet.
      expect(screen.getByRole("heading", { name: "acme-eu is signed out" })).toBeInTheDocument();
    });

    it("renames an integration from its menu", async () => {
      const user = userEvent.setup();
      const rename = vi.spyOn(api, "RenameIntegration").mockResolvedValue();
      renderDashboard();
      await pick(user, "acme", /rename/i);
      const dialog = await screen.findByRole("dialog", { name: /rename account/i });
      const input = within(dialog).getByRole("textbox");
      expect(input).toHaveValue("acme");
      expect(within(dialog).getByRole("button", { name: "Save" })).toBeDisabled();
      await user.clear(input);
      await user.type(input, "acme-corp");
      await user.click(within(dialog).getByRole("button", { name: "Save" }));
      await waitFor(() => expect(rename).toHaveBeenCalledWith("acme", "acme-corp"));
      await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    });

    it("offers the same actions on right click", async () => {
      const user = userEvent.setup();
      const remove = vi.spyOn(api, "RemoveIntegration").mockResolvedValue();
      renderDashboard();
      await user.pointer({ keys: "[MouseRight]", target: screen.getByRole("button", { name: "acme-eu" }) });
      const menu = await screen.findByRole("menu");
      expect(
        within(menu)
          .getAllByRole("menuitem")
          .map((m) => m.textContent?.trim()),
      ).toEqual(["Rename", "Sign in", "Remove"]);
      await user.click(within(menu).getByRole("menuitem", { name: /remove/i }));
      await waitFor(() => expect(remove).toHaveBeenCalledWith("acme-eu"));
    });
  });

  describe("adding", () => {
    // Each sidebar section has its own Add button; the header one comes last.
    const headerAdd = () => screen.getAllByRole("button", { name: "Add" }).slice(-1)[0];

    it.each([
      ["Import from this machine", "Import from this machine"],
      ["AWS Identity Center portal", "Add Identity Center portal"],
      ["AWS assume role", "Assume a role"],
      ["AWS IAM user access key", "Add IAM user"],
      ["Azure tenant", "Add Azure tenant"],
      ["Google Cloud account", "Add Google Cloud account"],
      ["GCP service account impersonation", "Impersonate a service account"],
    ])("opens %s from the Add menu", async (item, title) => {
      const user = userEvent.setup();
      renderDashboard();
      await user.click(headerAdd());
      await user.click(await screen.findByRole("menuitem", { name: item }));
      expect(await screen.findByRole("dialog", { name: title })).toBeInTheDocument();
    });

    it("opens the add dialog of each sidebar section", async () => {
      const user = userEvent.setup();
      renderDashboard();
      const sections = ["Add Identity Center portal", "Add Azure tenant", "Add Google Cloud account"];
      for (const [i, title] of sections.entries()) {
        // The sidebar buttons come first in document order; the header Add is last.
        await user.click(screen.getAllByRole("button", { name: "Add" })[i]);
        expect(await screen.findByRole("dialog", { name: title })).toBeInTheDocument();
        await user.keyboard("{Escape}");
        await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
      }
    });

    it("disables the entries that need an existing AWS session or Google account", async () => {
      const user = userEvent.setup();
      workspace = { ...workspace, sessions: [], integrations: [] } as Workspace;
      renderDashboard();
      await user.click(headerAdd());
      expect(await screen.findByRole("menuitem", { name: "AWS assume role" })).toHaveAttribute("aria-disabled", "true");
      expect(screen.getByRole("menuitem", { name: "GCP service account impersonation" })).toHaveAttribute(
        "aria-disabled",
        "true",
      );
      expect(screen.getByRole("menuitem", { name: "AWS Identity Center portal" })).not.toHaveAttribute("aria-disabled");
    });
  });

  describe("empty states", () => {
    it("guides a new workspace to import or connect", async () => {
      const user = userEvent.setup();
      workspace = { ...workspace, sessions: [], integrations: [] } as Workspace;
      renderDashboard();
      expect(screen.getByRole("heading", { name: "No sessions yet" })).toBeInTheDocument();
      expect(screen.getAllByText("None yet")).toHaveLength(3);
      expect(screen.getByText("0 active")).toBeInTheDocument();
      // The sidebar hides filters that have nothing to show.
      expect(screen.queryByRole("button", { name: /^Active/ })).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: /^Favorites/ })).not.toBeInTheDocument();
      await user.click(screen.getByRole("button", { name: /import from this machine/i }));
      expect(await screen.findByRole("dialog", { name: /import from this machine/i })).toBeInTheDocument();
      await user.keyboard("{Escape}");
      await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
      await user.click(screen.getByRole("button", { name: /connect aws identity center/i }));
      expect(await screen.findByRole("dialog", { name: /add identity center portal/i })).toBeInTheDocument();
    });

    it("offers a sync for a signed-in integration with no sessions", async () => {
      const user = userEvent.setup();
      workspace = {
        ...workspace,
        integrations: workspace.integrations.map((i) =>
          i.id === "acme-eu" ? { ...i, awsSso: { ...i.awsSso, tokenExpires: new Date().toISOString() } } : i,
        ),
      } as Workspace;
      const sync = vi.spyOn(api, "SyncSSO").mockResolvedValue([]);
      const success = vi.spyOn(toast, "success");
      renderDashboard();
      await user.click(screen.getByRole("button", { name: "acme-eu" }));
      expect(screen.getByRole("heading", { name: "No sessions in acme-eu" })).toBeInTheDocument();
      await user.click(screen.getByRole("button", { name: /sync acme-eu/i }));
      await waitFor(() => expect(success).toHaveBeenCalledWith("Synced"));
      expect(sync).toHaveBeenCalledWith("acme-eu");
    });

    it("opens the sign-in dialog from a signed-out integration's empty state", async () => {
      const user = userEvent.setup();
      vi.spyOn(api, "StartSSOLogin").mockReturnValue(new Promise(() => {}) as never);
      const cancel = vi.spyOn(api, "CancelSSOLogin").mockResolvedValue();
      renderDashboard();
      await user.click(screen.getByRole("button", { name: "acme-eu" }));
      await user.click(screen.getByRole("button", { name: /sign in to acme-eu/i }));
      expect(await screen.findByRole("dialog", { name: /sign in to acme-eu/i })).toBeInTheDocument();
      await user.keyboard("{Escape}");
      await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
      expect(cancel).toHaveBeenCalledWith("acme-eu");
    });

    it("says when a filter and search leave nothing", async () => {
      const user = userEvent.setup();
      renderDashboard();
      await user.click(screen.getByRole("button", { name: /^Favorites/ }));
      await user.type(screen.getByPlaceholderText("Search sessions"), "contoso");
      expect(screen.getByRole("heading", { name: "Nothing matches" })).toBeInTheDocument();
      expect(screen.getByText("Try a different search or filter.")).toBeInTheDocument();
    });

    it("keeps an integration's guidance when a search empties its list", async () => {
      const user = userEvent.setup();
      renderDashboard();
      await user.click(screen.getByRole("button", { name: "contoso" }));
      await user.type(screen.getByPlaceholderText("Search sessions"), "acme");
      expect(screen.getByRole("heading", { name: "No sessions in contoso" })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: /sync contoso/i })).toBeInTheDocument();
    });
  });
});
