import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
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
    // Account groups and standalone rows from the seed, each once: favorites
    // live in the sidebar filter, not in a panel of their own.
    expect(screen.getByText("Acme Prod")).toBeInTheDocument();
    expect(screen.getByText("Acme Dev")).toBeInTheDocument();
    expect(screen.getByText("Contoso Production")).toBeInTheDocument();
    expect(screen.getByText("data-platform")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: /favorites/i })).not.toBeInTheDocument();
    expect(screen.getAllByText("deployer")).toHaveLength(1);
    expect(screen.getByText("2 active")).toBeInTheDocument();
  });

  it("resizes the sidebar by dragging its edge and resets on double-click", () => {
    renderDashboard();
    const handle = screen.getByRole("separator", { name: "Resize sidebar" });
    const aside = handle.closest("aside")!;
    expect(aside.style.width).toBe("256px");
    fireEvent.mouseDown(handle, { clientX: 256 });
    fireEvent.mouseMove(window, { clientX: 316 });
    fireEvent.mouseUp(window);
    expect(aside.style.width).toBe("316px");
    expect(localStorage.getItem("rolle.sidebar")).toBe("316");
    fireEvent.doubleClick(handle);
    expect(aside.style.width).toBe("256px");
  });

  it("filters to active sessions", async () => {
    const user = userEvent.setup();
    renderDashboard();
    await user.click(screen.getByRole("button", { name: /^Active/ }));
    expect(rowNames()).toEqual(["Acme Prod", "AdministratorAccess", "Contoso Production"]);
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

    // The sidebar item's menu brings everything back at once.
    const unhide = vi.spyOn(api, "UnhideAll").mockResolvedValue();
    fireEvent.contextMenu(screen.getByRole("button", { name: /^Hidden/ }));
    await user.click(await screen.findByRole("menuitem", { name: "Unhide all" }));
    expect(unhide).toHaveBeenCalled();

    // A search names it even on the full list.
    await user.click(screen.getByRole("button", { name: /^All sessions/ }));
    await user.type(screen.getByPlaceholderText(/search/i), "personal");
    expect(rowNames()).toContain("personal");
  });

  it("lists tags in the sidebar, filters by one, and takes a dropped session", async () => {
    const user = userEvent.setup();
    // One hidden session, so the Hidden entry is there to drop on.
    Object.assign(workspace.sessions[0], { hidden: true });
    renderDashboard();
    // Seeded: Production with one session, Sandbox with none. Chips on rows
    // carry the same names, so the sidebar is queried on its own.
    const sidebar = within(screen.getByRole("complementary"));
    expect(sidebar.getByRole("button", { name: /^Production/ })).toHaveTextContent("1");
    expect(sidebar.getByRole("button", { name: /^Sandbox/ })).toHaveTextContent("0");
    await user.click(sidebar.getByRole("button", { name: /^Production/ }));
    expect(rowNames()).toEqual(["deployer"]);
    // The chip on the row leads to the same filter.
    await user.click(screen.getByRole("button", { name: /^All sessions/ }));
    await user.click(screen.getByRole("button", { name: /^Production$/ }));
    expect(rowNames()).toEqual(["deployer"]);

    const setTag = vi.spyOn(api, "SetSessionTag").mockResolvedValue();
    const data = new Map<string, string>([["application/x-rolle-session", "s-personal"]]);
    const dataTransfer = {
      types: Array.from(data.keys()),
      getData: (k: string) => data.get(k) ?? "",
      setData: (k: string, v: string) => void data.set(k, v),
      effectAllowed: "all",
    };
    const sandbox = sidebar.getByRole("button", { name: /^Sandbox/ }).closest("div")!;
    fireEvent.dragOver(sandbox, { dataTransfer });
    fireEvent.drop(sandbox, { dataTransfer });
    expect(setTag).toHaveBeenCalledWith("s-personal", "Sandbox", true);

    // A dragged tag shows a line on the side of the row the pointer is in,
    // and lands there. jsdom has no layout, so the row is a zero box at 0,0:
    // a positive clientY is the lower half.
    const move = vi.spyOn(api, "MoveTag").mockResolvedValue();
    const tagData = {
      ...dataTransfer,
      types: ["application/x-rolle-tag"],
      getData: (k: string) => (k === "application/x-rolle-tag" ? "Production" : ""),
    };
    fireEvent.dragOver(sandbox, { dataTransfer: tagData, clientY: 1 });
    expect(sandbox.querySelector("[data-drop-line]")).toHaveAttribute("data-drop-line", "after");
    fireEvent.drop(sandbox, { dataTransfer: tagData, clientY: 1 });
    expect(move).toHaveBeenCalledWith("Production", 1);
    expect(sandbox.querySelector("[data-drop-line]")).toBeNull();

    // Hidden takes a drop too.
    const hide = vi.spyOn(api, "SetHidden").mockResolvedValue();
    const hidden = sidebar.getByRole("button", { name: /^Hidden/ }).closest("div")!;
    fireEvent.dragOver(hidden, { dataTransfer });
    fireEvent.drop(hidden, { dataTransfer });
    expect(hide).toHaveBeenCalledWith("s-personal", true);

    // The + button opens the new-tag dialog, which saves through AddTag.
    const add = vi.spyOn(api, "AddTag").mockResolvedValue();
    await user.click(screen.getByRole("button", { name: "New tag" }));
    await user.type(await screen.findByPlaceholderText("Production"), "Staging");
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(add).toHaveBeenCalledWith(expect.objectContaining({ name: "Staging", color: "#8b9099", icon: "tag" })),
    );
  });

  it("filters to favorites", async () => {
    const user = userEvent.setup();
    renderDashboard();
    await user.click(screen.getByRole("button", { name: /^Favorites/ }));
    expect(rowNames()).toEqual(["Acme Prod", "AdministratorAccess", "deployer"]);
  });

  it("leaves hidden sessions out of the sidebar counts", () => {
    // deployer is a favorite and personal is the only IAM user; hidden, each
    // leaves its list and its count. Users has nothing left, so it goes.
    for (const name of ["deployer", "personal"]) {
      Object.assign(
        workspace.sessions.find((s) => s.name === name)!,
        { hidden: true },
      );
    }
    renderDashboard();
    const sidebar = within(screen.getByRole("complementary"));
    expect(sidebar.getByRole("button", { name: /^Favorites/ })).toHaveTextContent("Favorites1");
    expect(sidebar.queryByRole("button", { name: /^Users/ })).not.toBeInTheDocument();
    expect(sidebar.getByRole("button", { name: /^Assumed roles/ })).toHaveTextContent("Assumed roles1");
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
    expect(rowNames()).toEqual(["deployer"]);
    await user.clear(screen.getByPlaceholderText("Search sessions"));
    await user.type(screen.getByPlaceholderText("Search sessions"), "readonly");
    // A search keeps the matching account group visible with only the matching role.
    expect(rowNames()).toEqual(["Acme Prod", "ReadOnlyAccess"]);
    await user.clear(screen.getByPlaceholderText("Search sessions"));
    await user.type(screen.getByPlaceholderText("Search sessions"), "zzz");
    expect(screen.getByText("Nothing matches")).toBeInTheDocument();
  });

  it("hides the sidebar from the header button and the keyboard, and remembers it", async () => {
    const user = userEvent.setup();
    renderDashboard();
    expect(screen.getByRole("separator", { name: "Resize sidebar" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Hide sidebar" }));
    expect(screen.queryByRole("separator", { name: "Resize sidebar" })).not.toBeInTheDocument();
    expect(localStorage.getItem("rolle.sidebar.hidden")).toBe("1");
    // The header takes over Settings while the sidebar footer is away.
    expect(screen.getByRole("button", { name: "Settings" }).closest("header")).not.toBeNull();
    fireEvent.keyDown(window, { key: "\\", metaKey: true });
    expect(screen.getByRole("separator", { name: "Resize sidebar" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Show sidebar" })).not.toBeInTheDocument();
    expect(localStorage.getItem("rolle.sidebar.hidden")).toBe("0");
  });

  it("narrows rows with the filter chips and drops the picks when the row closes", async () => {
    const user = userEvent.setup();
    renderDashboard();
    expect(screen.queryByRole("toolbar", { name: "Filters" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Filters" }));
    await user.click(screen.getByRole("button", { name: "Filter by region" }));
    await user.click(await screen.findByRole("menuitemcheckbox", { name: "eu-west-1" }));
    // The open menu hides the page from assistive tech; close it before reading the table.
    await user.keyboard("{Escape}");
    expect(rowNames()).toEqual(["prod-admin"]);
    // The chip names its one pick.
    expect(screen.getByRole("button", { name: "Filter by region" })).toHaveTextContent("eu-west-1");
    await user.click(screen.getByRole("button", { name: "Clear" }));
    // Eight sessions and two account rows.
    expect(rowNames()).toHaveLength(10);
    await user.click(screen.getByRole("button", { name: "Filter by cloud" }));
    await user.click(await screen.findByRole("menuitemcheckbox", { name: "Google Cloud" }));
    await user.keyboard("{Escape}");
    expect(rowNames()).toEqual(["data-platform", "deployer"]);
    // Closing the row shows every session again.
    await user.click(screen.getByRole("button", { name: "Filters" }));
    expect(screen.queryByRole("toolbar", { name: "Filters" })).not.toBeInTheDocument();
    expect(rowNames()).toHaveLength(10);
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
    await user.keyboard("{Escape}");
    await user.click(screen.getByRole("button", { name: "Help" }));
    expect(await screen.findByRole("menuitem", { name: /documentation/i })).toBeInTheDocument();
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
      // Identity Center, AWS IAM, Azure, and Google Cloud each say so.
      expect(screen.getAllByText("None yet")).toHaveLength(4);
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

  describe("hidden sections", () => {
    it("drops hidden sections from the sidebar and from the shortcut order", async () => {
      const user = userEvent.setup();
      workspace = {
        ...workspace,
        settings: { ...workspace.settings, hiddenSections: ["aws-sso", "azure", "aws-iam"] },
      } as Workspace;
      renderDashboard();
      const sidebar = within(screen.getByRole("complementary"));
      for (const gone of ["AWS Identity Center", "Azure tenants", "acme", "contoso", "Users"]) {
        expect(sidebar.queryByText(gone)).not.toBeInTheDocument();
      }
      expect(sidebar.getByText("Google Cloud")).toBeInTheDocument();
      // With five filters ahead of it, the Google Cloud account is Ctrl+6;
      // in the full sidebar it sits past the ninth slot.
      await user.keyboard("{Control>}");
      await sidebar.findByText("Ctrl+6");
      // The badge sits next to its button, inside the item's wrapper.
      expect(sidebar.getByRole("button", { name: "gcp" }).parentElement).toHaveTextContent("Ctrl+6");
      await user.keyboard("6{/Control}");
      expect(rowNames()).toContain("data-platform");
      expect(rowNames()).not.toContain("Contoso Production");
    });

    it("keeps every section and the late shortcut slots when nothing is hidden", async () => {
      const user = userEvent.setup();
      renderDashboard();
      const sidebar = within(screen.getByRole("complementary"));
      expect(sidebar.getByText("AWS Identity Center")).toBeInTheDocument();
      expect(sidebar.getByText("Azure tenants")).toBeInTheDocument();
      await user.keyboard("{Control>}");
      await sidebar.findByText("Ctrl+6");
      expect(sidebar.getByRole("button", { name: /^acme$/ }).parentElement).toHaveTextContent("Ctrl+6");
      expect(sidebar.getByRole("button", { name: "gcp" }).parentElement).not.toHaveTextContent(/Ctrl\+/);
      await user.keyboard("{/Control}");
    });
  });

  describe("tag menus", () => {
    it("edits and removes a tag from its sidebar menu", async () => {
      const user = userEvent.setup();
      const remove = vi.spyOn(api, "RemoveTag").mockResolvedValue();
      const success = vi.spyOn(toast, "success");
      renderDashboard();
      const sidebar = within(screen.getByRole("complementary"));
      fireEvent.contextMenu(sidebar.getByRole("button", { name: /^Sandbox/ }));
      await user.click(await screen.findByRole("menuitem", { name: "Edit tag" }));
      const dialog = await screen.findByRole("dialog");
      expect(within(dialog).getByDisplayValue("Sandbox")).toBeInTheDocument();
      await user.keyboard("{Escape}");
      await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
      fireEvent.contextMenu(sidebar.getByRole("button", { name: /^Sandbox/ }));
      await user.click(await screen.findByRole("menuitem", { name: "Remove tag" }));
      await waitFor(() => expect(remove).toHaveBeenCalledWith("Sandbox"));
      expect(success).toHaveBeenCalledWith("Tag Sandbox removed");
    });

    it("says None yet without tags and adds an IAM user from the AWS section", async () => {
      const user = userEvent.setup();
      workspace = { ...workspace, tags: [] } as Workspace;
      renderDashboard();
      const sidebar = within(screen.getByRole("complementary"));
      expect(sidebar.getAllByText("None yet").length).toBeGreaterThan(0);
      await user.click(sidebar.getByTitle("Add an IAM user"));
      expect(await screen.findByRole("dialog", { name: "Add IAM user" })).toBeInTheDocument();
    });
  });

  it("reports a failed update install, but not a cancelled one", async () => {
    const user = userEvent.setup();
    window.history.replaceState({}, "", "/?view=dashboard&update=1");
    const install = vi.spyOn(api, "InstallUpdate").mockRejectedValueOnce(new Error("cancelled"));
    const error = vi.spyOn(toast, "error");
    renderDashboard();
    const button = await screen.findByRole("button", { name: /update to 0\.2\.0/i }, { timeout: 3000 });
    await user.click(button);
    await waitFor(() => expect(install).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(screen.getByRole("button", { name: /update to 0\.2\.0/i })).toBeEnabled());
    expect(error).not.toHaveBeenCalled();
    install.mockRejectedValueOnce(new Error("signature mismatch"));
    await user.click(screen.getByRole("button", { name: /update to 0\.2\.0/i }));
    await waitFor(() => expect(error).toHaveBeenCalledWith("signature mismatch"));
  });
});
