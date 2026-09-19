import { fireEvent, render, renderHook, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@/components/ui/tooltip";
import { SessionTable, useColumnWidths, type ColumnWidths } from "@/components/dashboard/SessionTable";
import { api, Kind, type Session, type Workspace } from "@/lib/api";
import { integration, session, ssoRole } from "@/test/fixtures";

const widths: ColumnWidths = { profile: 150, region: 130, state: 130 };

function workspaceWith(sessions: Session[]): Workspace {
  return {
    version: 1,
    onboarded: true,
    sessions,
    integrations: [integration({ id: "acme", alias: "acme" })],
  } as unknown as Workspace;
}

function renderTable(sessions: Session[], props: Partial<React.ComponentProps<typeof SessionTable>> = {}) {
  const ws = workspaceWith(sessions);
  return render(
    <TooltipProvider>
      <SessionTable
        sessions={sessions}
        workspace={ws}
        widths={widths}
        onWidths={vi.fn()}
        onNeedsLogin={vi.fn()}
        {...props}
      />
    </TooltipProvider>,
  );
}

const bodyRows = () => within(screen.getAllByRole("rowgroup")[1]).getAllByRole("row");

describe("SessionTable", () => {
  const roles = [ssoRole("Acme Prod", "111", "ReadOnly"), ssoRole("Acme Prod", "111", "Admin")];
  const iam = session({ name: "personal", kind: Kind.KindAWSIAMUser });

  it("renders an account row with nested role rows", () => {
    renderTable([...roles, iam]);
    expect(screen.getByText("Acme Prod")).toBeInTheDocument();
    expect(screen.getByText(/111 · 2 roles/)).toBeInTheDocument();
    expect(screen.getByText("Admin")).toBeInTheDocument();
    expect(screen.getByText("ReadOnly")).toBeInTheDocument();
    expect(screen.getByText("personal")).toBeInTheDocument();
    // Account, two roles, one standalone.
    expect(bodyRows()).toHaveLength(4);
    // Nested rows show the role name, not the full "account/role" name.
    expect(screen.queryByText("Acme Prod/Admin")).not.toBeInTheDocument();
  });

  it("hides and unhides a whole account from its context menu", async () => {
    const user = userEvent.setup();
    const hide = vi.spyOn(api, "SetAccountHidden").mockResolvedValue();
    const view = renderTable(roles);
    fireEvent.contextMenu(screen.getByText("Acme Prod"));
    await user.click(await screen.findByRole("menuitem", { name: "Hide account" }));
    expect(hide).toHaveBeenCalledWith("acme", "111", true);
    view.unmount();

    renderTable(roles.map((r) => ({ ...r, hidden: true })));
    fireEvent.contextMenu(screen.getByText("Acme Prod"));
    await user.click(await screen.findByRole("menuitem", { name: "Unhide account" }));
    expect(hide).toHaveBeenCalledWith("acme", "111", false);
  });

  it("collapses an account when its row is clicked and remembers it", async () => {
    const user = userEvent.setup();
    renderTable(roles);
    await user.click(screen.getByText("Acme Prod"));
    expect(screen.queryByText("Admin")).not.toBeInTheDocument();
    expect(screen.queryByText("ReadOnly")).not.toBeInTheDocument();
    expect(bodyRows()).toHaveLength(1);
    expect(JSON.parse(localStorage.getItem("rolle.collapsed") ?? "{}")).toEqual({ keys: ["acme:111"] });
    await user.click(screen.getByText("Acme Prod"));
    expect(screen.getByText("Admin")).toBeInTheDocument();
  });

  it("keeps collapsed accounts open while searching", () => {
    localStorage.setItem("rolle.collapsed", JSON.stringify({ keys: ["acme:111"] }));
    const first = renderTable(roles);
    expect(screen.queryByText("Admin")).not.toBeInTheDocument();
    first.unmount();
    renderTable(roles, { searching: true });
    expect(screen.getByText("Admin")).toBeInTheDocument();
    expect(screen.getByText("ReadOnly")).toBeInTheDocument();
  });

  it("sorts by a column on header click, reverses, then clears", async () => {
    const user = userEvent.setup();
    renderTable([
      session({ name: "alpha", aws: { profile: "zed" } as Session["aws"] }),
      session({ name: "bravo", aws: { profile: "yak" } as Session["aws"] }),
      session({ name: "charlie", aws: { profile: "xi" } as Session["aws"] }),
    ]);
    const names = () => bodyRows().map((r) => r.querySelector("p")?.textContent);
    expect(names()).toEqual(["alpha", "bravo", "charlie"]);
    const profile = screen.getByRole("button", { name: "Profile" });
    await user.click(profile);
    expect(names()).toEqual(["charlie", "bravo", "alpha"]);
    expect(screen.getByRole("columnheader", { name: /Profile/ })).toHaveAttribute("aria-sort", "ascending");
    await user.click(profile);
    expect(names()).toEqual(["alpha", "bravo", "charlie"]);
    expect(JSON.parse(localStorage.getItem("rolle.sort")!)).toEqual({ sort: { key: "profile", dir: "desc" } });
    await user.click(profile);
    expect(screen.getByRole("columnheader", { name: /Profile/ })).toHaveAttribute("aria-sort", "none");
    localStorage.removeItem("rolle.sort");
  });

  it("drags a divider by trading width between its two columns", () => {
    const onWidths = vi.fn();
    renderTable([session({ name: "alpha" })], { onWidths });
    const between = screen.getByRole("separator", { name: "Resize Profile column" });
    fireEvent.mouseDown(between, { clientX: 100 });
    fireEvent.mouseMove(window, { clientX: 120 });
    fireEvent.mouseUp(window);
    expect(onWidths).toHaveBeenLastCalledWith({ profile: 170, region: 110, state: 130 });
    // The Session divider only sets Profile; Session itself is the flexible column.
    const first = screen.getByRole("separator", { name: "Resize Session column" });
    fireEvent.mouseDown(first, { clientX: 100 });
    fireEvent.mouseMove(window, { clientX: 70 });
    fireEvent.mouseUp(window);
    expect(onWidths).toHaveBeenLastCalledWith({ profile: 180, region: 130, state: 130 });
    // Neither column leaves its range: Region stops at its 90px floor, so Profile stops at 190.
    fireEvent.mouseDown(between, { clientX: 100 });
    fireEvent.mouseMove(window, { clientX: 400 });
    fireEvent.mouseUp(window);
    expect(onWidths).toHaveBeenLastCalledWith({ profile: 190, region: 90, state: 130 });
    expect(screen.queryByRole("separator", { name: "Resize State column" })).toBeNull();
  });

  it("keeps the Session column at least 160px wide", () => {
    const onWidths = vi.fn();
    renderTable([session({ name: "alpha" })], { onWidths });
    const first = screen.getByRole("separator", { name: "Resize Session column" });
    Object.defineProperty(first.parentElement, "clientWidth", { value: 200 });
    fireEvent.mouseDown(first, { clientX: 100 });
    fireEvent.mouseMove(window, { clientX: 0 });
    fireEvent.mouseUp(window);
    expect(onWidths).toHaveBeenLastCalledWith({ profile: 190, region: 130, state: 130 });
  });

  it("pulls stored widths from older builds into range", () => {
    localStorage.setItem("rolle.columns", JSON.stringify({ profile: 400, region: 70, state: 70 }));
    const { result } = renderHook(() => useColumnWidths());
    expect(result.current[0]).toEqual({ profile: 320, region: 90, state: 120 });
    localStorage.removeItem("rolle.columns");
  });

  it("takes the Profile and Region column widths from the widths prop", () => {
    const { container } = renderTable([iam], { widths: { profile: 210, region: 95, state: 130 } });
    const cols = container.querySelectorAll("col");
    expect(cols).toHaveLength(6);
    expect(cols[2].style.width).toBe("210px");
    expect(cols[3].style.width).toBe("95px");
    expect(cols[4].style.width).toBe("130px");
  });
});

describe("SessionTable appearance", () => {
  const roles = [ssoRole("Acme Prod", "111", "ReadOnly"), ssoRole("Acme Prod", "111", "Admin")];
  const withSettings = (sessions: Session[], settings: Partial<NonNullable<Workspace["settings"]>>) =>
    ({ ...workspaceWith(sessions), settings }) as Workspace;

  it("hides a column from the picker, drops its sort, and remembers it", async () => {
    const user = userEvent.setup();
    localStorage.setItem("rolle.sort", JSON.stringify({ sort: { key: "profile", dir: "asc" } }));
    const { container } = renderTable([session({ name: "alpha", aws: { profile: "zed" } as Session["aws"] })]);
    expect(container.querySelectorAll("col")).toHaveLength(6);
    fireEvent.contextMenu(screen.getByRole("columnheader", { name: /Session/ }));
    await user.click(await screen.findByRole("menuitemcheckbox", { name: "Profile" }));
    // The open menu hides the page from assistive tech; close it before reading the table.
    await user.keyboard("{Escape}");
    expect(screen.getByRole("columnheader", { name: /Region/ })).toBeInTheDocument();
    expect(screen.queryByRole("columnheader", { name: /Profile/ })).not.toBeInTheDocument();
    expect(container.querySelectorAll("col")).toHaveLength(5);
    expect(screen.queryByText("zed")).not.toBeInTheDocument();
    // The Session divider now sets Region, the first column on show; Profile has none.
    expect(screen.getByRole("separator", { name: "Resize Session column" })).toBeInTheDocument();
    expect(screen.queryByRole("separator", { name: "Resize Profile column" })).toBeNull();
    expect(JSON.parse(localStorage.getItem("rolle.columns.shown")!)).toEqual({
      profile: false,
      region: true,
      state: true,
    });
    expect(JSON.parse(localStorage.getItem("rolle.sort")!)).toEqual({ sort: null });
  });

  it("stretches the account row across every column", () => {
    renderTable(roles);
    // Session, three optional columns, and Actions.
    expect(screen.getByText("Acme Prod").closest("td")).toHaveAttribute("colspan", "5");
  });

  it("lists every role on its own row when grouping is off", () => {
    renderTable(roles, { workspace: withSettings(roles, { flatList: true }) });
    expect(screen.queryByText(/2 roles/)).not.toBeInTheDocument();
    expect(screen.getByText("Acme Prod/Admin")).toBeInTheDocument();
    expect(bodyRows()).toHaveLength(2);
  });

  it("draws one line per row in compact mode", () => {
    const iam = session({ name: "personal", kind: Kind.KindAWSIAMUser, region: "us-west-2" });
    renderTable([...roles, iam], { workspace: withSettings([...roles, iam], { compact: true }) });
    // The standalone row loses its subtitle; the account row keeps its ID next to the name.
    expect(screen.getByText("personal")).toBeInTheDocument();
    expect(screen.queryByText("us-west-2", { selector: "p" })).not.toBeInTheDocument();
    expect(screen.getByText(/111 · 2 roles/)).toBeInTheDocument();
  });
});
