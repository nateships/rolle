import { fireEvent, render, renderHook, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@/components/ui/tooltip";
import { SessionTable, useColumnWidths, type SetWidths } from "@/components/dashboard/SessionTable";
import { api, Kind, type Session, type Workspace } from "@/lib/api";
import { integration, session, ssoRole } from "@/test/fixtures";

// Every column has a width that the user set, so no column fits its content.
const widths: SetWidths = { profile: 150, region: 130, state: 130 };

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

  it("reads a stored default width as a column that the user did not set", () => {
    expect(renderHook(() => useColumnWidths()).result.current[0]).toEqual({});
    localStorage.setItem("rolle.columns", JSON.stringify({ profile: 150, region: 200, state: 130 }));
    expect(renderHook(() => useColumnWidths()).result.current[0]).toEqual({ region: 200 });
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

// jsdom has no layout. Give each element 10px for each character, and put the
// header cells side by side, 100px each.
function mockLayout(tableWidth: number) {
  vi.spyOn(Element.prototype, "clientWidth", "get").mockReturnValue(tableWidth);
  vi.spyOn(Element.prototype, "getBoundingClientRect").mockImplementation(function (this: Element) {
    const header = this.matches("th");
    const left = header ? [...this.parentElement!.children].indexOf(this) * 100 : 0;
    return { left, width: header ? 100 : (this.textContent ?? "").length * 10 } as DOMRect;
  });
}

describe("SessionTable column fit", () => {
  const long = session({
    name: "alpha",
    region: "us-east-1",
    aws: { profile: "timescale-dev-platform" } as Session["aws"],
  });
  const colWidths = (container: HTMLElement) =>
    [...container.querySelectorAll("col")].slice(2, 5).map((c) => c.style.width);

  it("grows a column until its longest cell shows in full", () => {
    mockLayout(1200);
    const { container } = renderTable([long, session({ name: "bravo" })], { widths: {} });
    // 22 characters need 220px, and a jsdom cell has 1px of padding on each
    // side. The other columns keep their default width.
    expect(colWidths(container)).toEqual(["222px", "130px", "130px"]);
  });

  it("keeps the default width without a layout", () => {
    const { container } = renderTable([long], { widths: {} });
    expect(colWidths(container)).toEqual(["150px", "130px", "130px"]);
  });

  it("does not fit a column that the user set", () => {
    mockLayout(1200);
    const { container } = renderTable([long], { widths: { profile: 100 } });
    expect(colWidths(container)).toEqual(["100px", "130px", "130px"]);
  });

  it("stops the growth when the Session column is down to 160px", () => {
    // 84 + 184 for the fixed columns, 160 for Session, 410 for the defaults: 30px is left.
    mockLayout(868);
    const { container } = renderTable([long], { widths: {} });
    expect(colWidths(container)).toEqual(["180px", "130px", "130px"]);
  });

  it("starts a drag from the fitted width and sets only the two columns at the divider", () => {
    mockLayout(1200);
    const onWidths = vi.fn();
    renderTable([long], { widths: {}, onWidths });
    const between = screen.getByRole("separator", { name: "Resize Profile column" });
    fireEvent.mouseDown(between, { clientX: 100 });
    fireEvent.mouseMove(window, { clientX: 120 });
    fireEvent.mouseUp(window);
    expect(onWidths).toHaveBeenLastCalledWith({ profile: 242, region: 110 });
  });

  it("fits both columns at a divider again on double-click", () => {
    const onWidths = vi.fn();
    renderTable([long], { widths: { profile: 200, region: 100, state: 140 }, onWidths });
    fireEvent.doubleClick(screen.getByRole("separator", { name: "Resize Profile column" }));
    expect(onWidths).toHaveBeenLastCalledWith({ state: 140 });
    fireEvent.doubleClick(screen.getByRole("separator", { name: "Resize Session column" }));
    expect(onWidths).toHaveBeenLastCalledWith({ region: 100, state: 140 });
  });
});

describe("SessionTable column order", () => {
  const row = session({ name: "alpha", region: "us-east-1", aws: { profile: "zed" } as Session["aws"] });
  const headers = () => screen.getAllByRole("columnheader").map((h) => h.textContent);
  const header = (name: RegExp) => screen.getByRole("columnheader", { name });

  it("moves a column when its header is dragged past the middle of a neighbor", () => {
    mockLayout(1200);
    renderTable([row]);
    expect(headers()).toEqual(["", "Session", "Profile", "Region", "State", "Actions"]);
    // Profile is at 200-300 and Region at 300-400.
    fireEvent.mouseDown(header(/Profile/), { clientX: 250 });
    fireEvent.mouseMove(window, { clientX: 340 });
    expect(headers()).toEqual(["", "Session", "Profile", "Region", "State", "Actions"]);
    fireEvent.mouseMove(window, { clientX: 360 });
    expect(headers()).toEqual(["", "Session", "Region", "Profile", "State", "Actions"]);
    // Profile is now at 300-400. The pointer is not past the middle of State.
    fireEvent.mouseMove(window, { clientX: 440 });
    expect(headers()).toEqual(["", "Session", "Region", "Profile", "State", "Actions"]);
    fireEvent.mouseUp(window);
    expect(JSON.parse(localStorage.getItem("rolle.columns.order")!)).toEqual({ order: ["region", "profile", "state"] });
    // The cells follow the headers.
    const cells = within(bodyRows()[0]).getAllByRole("cell");
    expect(cells[2]).toHaveTextContent("us-east-1");
    expect(cells[3]).toHaveTextContent("zed");
  });

  it("moves a column to the left, and not past the Session column", () => {
    mockLayout(1200);
    renderTable([row]);
    fireEvent.mouseDown(header(/State/), { clientX: 450 });
    fireEvent.mouseMove(window, { clientX: 340 });
    fireEvent.mouseMove(window, { clientX: 240 });
    fireEvent.mouseMove(window, { clientX: 100 });
    fireEvent.mouseUp(window);
    expect(headers()).toEqual(["", "Session", "State", "Profile", "Region", "Actions"]);
  });

  it("does not sort after a drag, and still sorts on a click", () => {
    mockLayout(1200);
    renderTable([row]);
    const button = screen.getByRole("button", { name: "Profile" });
    fireEvent.mouseDown(button, { clientX: 250 });
    fireEvent.mouseMove(window, { clientX: 360 });
    fireEvent.mouseUp(window);
    fireEvent.click(button);
    expect(header(/Profile/)).toHaveAttribute("aria-sort", "none");
    // A press that does not move is a click.
    fireEvent.mouseDown(button, { clientX: 350 });
    fireEvent.mouseMove(window, { clientX: 352 });
    fireEvent.mouseUp(window);
    fireEvent.click(button);
    expect(header(/Profile/)).toHaveAttribute("aria-sort", "ascending");
    expect(headers()).toEqual(["", "Session", "Region", "Profile", "State", "Actions"]);
  });

  it("does not move a column when a divider is dragged", () => {
    mockLayout(1200);
    renderTable([row]);
    fireEvent.mouseDown(screen.getByRole("separator", { name: "Resize Profile column" }), { clientX: 300 });
    fireEvent.mouseMove(window, { clientX: 380 });
    fireEvent.mouseUp(window);
    expect(headers()).toEqual(["", "Session", "Profile", "Region", "State", "Actions"]);
  });

  it("reads the stored order, and ignores one that is not valid", () => {
    localStorage.setItem("rolle.columns.order", JSON.stringify({ order: ["state", "profile", "region"] }));
    const first = renderTable([row]);
    expect(headers()).toEqual(["", "Session", "State", "Profile", "Region", "Actions"]);
    first.unmount();
    localStorage.setItem("rolle.columns.order", JSON.stringify({ order: ["state", "state", "nope"] }));
    renderTable([row]);
    expect(headers()).toEqual(["", "Session", "Profile", "Region", "State", "Actions"]);
  });
});
