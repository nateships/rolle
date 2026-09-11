import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@/components/ui/tooltip";
import { SessionTable, type ColumnWidths } from "@/components/dashboard/SessionTable";
import { Kind, type Session, type Workspace } from "@/lib/api";
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

  it("renders no account rows when flat", () => {
    renderTable(roles, { flat: true });
    expect(screen.queryByText("Acme Prod")).not.toBeInTheDocument();
    expect(screen.queryByText(/2 roles/)).not.toBeInTheDocument();
    expect(bodyRows()).toHaveLength(2);
    // Flat rows carry the full session name.
    expect(screen.getByText("Acme Prod/Admin")).toBeInTheDocument();
    expect(screen.getByText("Acme Prod/ReadOnly")).toBeInTheDocument();
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
