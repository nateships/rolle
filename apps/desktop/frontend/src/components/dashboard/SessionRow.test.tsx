import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@/components/ui/tooltip";
import { SessionRow } from "@/components/dashboard/SessionRow";
import { api, Kind, Status, type Integration, type Session, type Workspace, type Tag } from "@/lib/api";
import { celebrate } from "@/lib/celebrate";
import { integration, session, ssoRole } from "@/test/fixtures";

// Confetti draws on a canvas that jsdom does not have.
vi.mock("@/lib/celebrate", () => ({ celebrate: vi.fn() }));

function renderRow(
  s: Session,
  opts: {
    integrations?: Integration[];
    tags?: Tag[];
    onNeedsLogin?: (i: Integration, id?: string) => void;
    onTagClick?: (tag: string) => void;
  } = {},
) {
  const ws = {
    version: 1,
    onboarded: true,
    sessions: [s],
    integrations: opts.integrations ?? [],
    tags: opts.tags ?? [],
  } as unknown as Workspace;
  return render(
    <TooltipProvider>
      <table>
        <tbody>
          <SessionRow session={s} workspace={ws} onNeedsLogin={opts.onNeedsLogin} onTagClick={opts.onTagClick} />
        </tbody>
      </table>
    </TooltipProvider>,
  );
}

const activeIAM = () =>
  session({
    name: "personal",
    kind: Kind.KindAWSIAMUser,
    status: Status.StatusActive,
    expires: new Date(Date.now() + 3600e3).toISOString(),
  });

describe("SessionRow", () => {
  it("offers Start for an inactive session", () => {
    renderRow(session({ name: "personal", kind: Kind.KindAWSIAMUser }));
    expect(screen.getByRole("button", { name: /start/i })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /stop/i })).not.toBeInTheDocument();
    expect(screen.getByText("Inactive")).toBeInTheDocument();
  });

  it("offers Stop and a countdown for an active session", () => {
    renderRow(
      session({
        name: "personal",
        kind: Kind.KindAWSIAMUser,
        status: Status.StatusActive,
        expires: new Date(Date.now() + 3600e3 + 5 * 60e3 + 1e3).toISOString(),
      }),
    );
    expect(screen.getByRole("button", { name: /stop/i })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^start$/i })).not.toBeInTheDocument();
    expect(screen.getByText("Active")).toBeInTheDocument();
    expect(screen.getByText("1h 5m")).toBeInTheDocument();
  });

  it("shows a kind badge only for IAM users and assume roles", () => {
    const iam = renderRow(session({ name: "personal", kind: Kind.KindAWSIAMUser }));
    expect(screen.getByText("IAM user")).toBeInTheDocument();
    iam.unmount();

    const assume = renderRow(
      session({ name: "prod-admin", kind: Kind.KindAWSAssumeRole, aws: { roleArn: "arn:aws:iam::1:role/x" } }),
    );
    expect(screen.getByText("Assume role")).toBeInTheDocument();
    assume.unmount();

    const sso = renderRow(ssoRole("Acme", "111", "Admin"));
    expect(screen.queryByText("SSO role")).not.toBeInTheDocument();
    sso.unmount();

    renderRow(session({ name: "Contoso", kind: Kind.KindAzure, azure: { subscriptionId: "sub", tenantId: "t" } }));
    expect(screen.queryByText("Azure")).not.toBeInTheDocument();
  });

  it("shows the profile and region only for AWS sessions", () => {
    const aws = renderRow(
      session({ name: "personal", kind: Kind.KindAWSIAMUser, region: "us-west-2", aws: { profile: "me" } }),
    );
    expect(screen.getByRole("button", { name: "me" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "us-west-2" })).toBeInTheDocument();
    aws.unmount();

    renderRow(session({ name: "data", kind: Kind.KindGCP, gcp: { projectId: "proj" } }));
    expect(screen.getAllByText("—")).toHaveLength(2);
  });

  it("opens the same actions from a right-click", async () => {
    renderRow(session({ name: "personal", kind: Kind.KindAWSIAMUser }));
    fireEvent.contextMenu(screen.getByText("personal"));
    expect(await screen.findByRole("menuitem", { name: /^start$/i })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: /rename/i })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: /change region/i })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: /remove$/i })).toBeInTheDocument();
  });

  it("shows a nested row with the role name only and the source of a chained role", () => {
    const source = session({ name: "base", kind: Kind.KindAWSIAMUser });
    const chained = session({
      name: "prod-admin",
      kind: Kind.KindAWSAssumeRole,
      aws: { roleArn: "arn:aws:iam::1:role/x", sourceSessionId: source.id },
    });
    const ws = { version: 1, onboarded: true, sessions: [source, chained], integrations: [] } as unknown as Workspace;
    render(
      <TooltipProvider>
        <table>
          <tbody>
            <SessionRow session={chained} workspace={ws} />
            <SessionRow session={ssoRole("Acme", "111", "Admin")} workspace={ws} nested />
          </tbody>
        </table>
      </TooltipProvider>,
    );
    expect(screen.getByText(/via base/)).toBeInTheDocument();
    expect(screen.getByText("Admin")).toBeInTheDocument();
    expect(screen.queryByText("Acme/Admin")).not.toBeInTheDocument();
  });
});

describe("SessionRow actions", () => {
  it("starts a session and offers to copy the env", async () => {
    const user = userEvent.setup();
    const start = vi.spyOn(api, "Start").mockResolvedValue({} as never);
    const success = vi.spyOn(toast, "success");
    const s = session({ name: "personal", kind: Kind.KindAWSIAMUser });
    renderRow(s);

    await user.click(screen.getByRole("button", { name: "Start" }));

    await waitFor(() => expect(start).toHaveBeenCalledWith(s.id, ""));
    expect(success).toHaveBeenCalledWith(
      "personal started",
      expect.objectContaining({ description: "AWS profile default is ready." }),
    );
    // The first active session gets a small celebration.
    expect(celebrate).toHaveBeenCalledWith("small");
  });

  it("hands off to the login flow when the backend needs a browser sign-in", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "Start").mockRejectedValue(new Error("login required: token expired"));
    const error = vi.spyOn(toast, "error");
    const acme = integration({ id: "acme", alias: "acme" });
    const onNeedsLogin = vi.fn();
    const s = ssoRole("Acme", "111", "Admin");
    renderRow(s, { integrations: [acme], onNeedsLogin });

    await user.click(screen.getByRole("button", { name: "Start" }));

    await waitFor(() => expect(onNeedsLogin).toHaveBeenCalledWith(acme, s.id));
    expect(error).not.toHaveBeenCalled();
  });

  it("shows other start errors as a toast", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "Start").mockRejectedValue(new Error("no credentials"));
    const error = vi.spyOn(toast, "error");
    renderRow(session({ name: "personal", kind: Kind.KindAWSIAMUser }));

    await user.click(screen.getByRole("button", { name: "Start" }));

    await waitFor(() => expect(error).toHaveBeenCalledWith("no credentials"));
    expect(screen.getByRole("button", { name: "Start" })).toBeEnabled();
  });

  it("asks for an MFA code first when the IAM user has a device", async () => {
    const user = userEvent.setup();
    const start = vi.spyOn(api, "Start").mockResolvedValue({} as never);
    const s = session({ name: "personal", kind: Kind.KindAWSIAMUser, aws: { mfaDevice: "arn:aws:iam::1:mfa/me" } });
    renderRow(s);

    await user.click(screen.getByRole("button", { name: "Start" }));
    const dialog = await screen.findByRole("dialog", { name: "MFA code" });
    expect(start).not.toHaveBeenCalled();

    await user.type(screen.getByPlaceholderText("000000"), "123456");
    await user.click(screen.getByRole("button", { name: "Start session" }));

    await waitFor(() => expect(start).toHaveBeenCalledWith(s.id, "123456"));
    await waitFor(() => expect(dialog).not.toBeInTheDocument());
  });

  it("stops an active session", async () => {
    const user = userEvent.setup();
    const stop = vi.spyOn(api, "Stop").mockResolvedValue();
    const s = activeIAM();
    renderRow(s);

    await user.click(screen.getByRole("button", { name: "Stop" }));
    await waitFor(() => expect(stop).toHaveBeenCalledWith(s.id));
  });

  it("shows a stop error as a toast", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "Stop").mockRejectedValue(new Error("stop failed"));
    const error = vi.spyOn(toast, "error");
    renderRow(activeIAM());

    await user.click(screen.getByRole("button", { name: "Stop" }));
    await waitFor(() => expect(error).toHaveBeenCalledWith("stop failed"));
  });

  it("hides and unhides from the context menu", async () => {
    const user = userEvent.setup();
    const hide = vi.spyOn(api, "SetHidden").mockResolvedValue();
    const plain = session({ name: "personal", kind: Kind.KindAWSIAMUser });
    const view = renderRow(plain);
    fireEvent.contextMenu(screen.getByText("personal"));
    await user.click(await screen.findByRole("menuitem", { name: /^hide$/i }));
    expect(hide).toHaveBeenCalledWith(plain.id, true);
    view.unmount();

    renderRow({ ...plain, hidden: true });
    expect(screen.getByLabelText("Hidden")).toBeInTheDocument();
    fireEvent.contextMenu(screen.getByText("personal"));
    await user.click(await screen.findByRole("menuitem", { name: /^unhide$/i }));
    expect(hide).toHaveBeenCalledWith(plain.id, false);
  });

  it("tags and untags from the Tags submenu and shows the chips", async () => {
    const user = userEvent.setup();
    const setTag = vi.spyOn(api, "SetSessionTag").mockResolvedValue();
    const plain = session({ name: "personal", kind: Kind.KindAWSIAMUser, tags: ["Production"] });
    const onTagClick = vi.fn();
    renderRow(plain, {
      tags: [{ name: "Production", color: "#e5484d", icon: "shield" }, { name: "Sandbox" }] as Tag[],
      onTagClick,
    });
    // The chip's name opens that tag's filter.
    await user.click(screen.getByRole("button", { name: /^Production$/ }));
    expect(onTagClick).toHaveBeenCalledWith("Production");
    // A submenu opens on hover, or with the right arrow from its trigger.
    // The chip's x takes the tag off.
    await user.click(screen.getByRole("button", { name: "Remove tag Production" }));
    expect(setTag).toHaveBeenCalledWith(plain.id, "Production", false);
    setTag.mockClear();

    // Keyboard: open the Tags submenu with the right arrow, move to Sandbox, choose it.
    fireEvent.contextMenu(screen.getByText("personal"));
    await user.hover(await screen.findByRole("menuitem", { name: /^tags$/i }));
    await user.keyboard("{ArrowRight}");
    await screen.findByRole("menuitem", { name: /sandbox/i });
    await user.keyboard("{ArrowDown}{Enter}");
    await waitFor(() => expect(setTag).toHaveBeenCalledWith(plain.id, "Sandbox", true));
    fireEvent.contextMenu(screen.getByText("personal"));
    await user.hover(await screen.findByRole("menuitem", { name: /^tags$/i }));
    await user.keyboard("{ArrowRight}");
    await screen.findByRole("menuitem", { name: /production/i });
    await user.keyboard("{Enter}");
    await waitFor(() => expect(setTag).toHaveBeenCalledWith(plain.id, "Production", false));
  });

  it("toggles the favorite star", async () => {
    const user = userEvent.setup();
    const fav = vi.spyOn(api, "SetFavorite").mockResolvedValue();
    const plain = session({ name: "personal", kind: Kind.KindAWSIAMUser });
    const view = renderRow(plain);
    await user.click(screen.getByRole("button", { name: "Add to favorites" }));
    expect(fav).toHaveBeenCalledWith(plain.id, true);
    view.unmount();

    const starred = session({ name: "personal", kind: Kind.KindAWSIAMUser, favorite: true });
    renderRow(starred);
    await user.click(screen.getByRole("button", { name: "Remove from favorites" }));
    expect(fav).toHaveBeenCalledWith(starred.id, false);
  });

  it("copies the credentials as env for an active session", async () => {
    const user = userEvent.setup();
    const s = activeIAM();
    const env = vi.spyOn(api, "EnvText").mockResolvedValue("export AWS_ACCESS_KEY_ID=X\n");
    const success = vi.spyOn(toast, "success");
    renderRow(s);

    await user.click(screen.getByRole("button", { name: "Copy credentials as env" }));

    await waitFor(() => expect(success).toHaveBeenCalledWith("Credentials copied", expect.anything()));
    expect(env).toHaveBeenCalledWith(s.id);
    expect(await navigator.clipboard.readText()).toBe("export AWS_ACCESS_KEY_ID=X\n");
  });

  it("copies the profile command", async () => {
    const user = userEvent.setup();
    const success = vi.spyOn(toast, "success");
    renderRow(session({ ...activeIAM(), aws: { profile: "work" } }));

    await user.click(screen.getByRole("button", { name: "Copy profile command" }));

    await waitFor(() => expect(success).toHaveBeenCalledWith("Profile command copied", expect.anything()));
    expect(await navigator.clipboard.readText()).toBe("aws --profile work");
  });

  it("shows a copy failure as a toast", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "EnvText").mockRejectedValue(new Error("session expired"));
    const error = vi.spyOn(toast, "error");
    renderRow(activeIAM());

    await user.click(screen.getByRole("button", { name: "Copy credentials as env" }));
    await waitFor(() => expect(error).toHaveBeenCalledWith("session expired"));
  });

  it("opens the console and a terminal for an active session", async () => {
    const user = userEvent.setup();
    const s = activeIAM();
    const openConsole = vi.spyOn(api, "OpenConsole").mockResolvedValue();
    const terminal = vi.spyOn(api, "OpenTerminal").mockResolvedValue();
    renderRow(s);

    await user.click(screen.getByRole("button", { name: "Open console" }));
    expect(openConsole).toHaveBeenCalledWith(s.id);
    await user.click(screen.getByRole("button", { name: "Open terminal here" }));
    expect(terminal).toHaveBeenCalledWith(s.id);
  });

  it("shows an open console failure as a toast", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "OpenConsole").mockRejectedValue(new Error("no browser"));
    const error = vi.spyOn(toast, "error");
    renderRow(activeIAM());

    await user.click(screen.getByRole("button", { name: "Open console" }));
    await waitFor(() => expect(error).toHaveBeenCalledWith("no browser"));
  });

  it("hides the credential actions for an inactive session", () => {
    renderRow(session({ name: "personal", kind: Kind.KindAWSIAMUser }));
    expect(screen.queryByRole("button", { name: "Open console" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Copy credentials as env" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "More actions" })).toBeInTheDocument();
  });

  it("lists the active-only items in the context menu and stops from there", async () => {
    const user = userEvent.setup();
    const s = activeIAM();
    const stop = vi.spyOn(api, "Stop").mockResolvedValue();
    renderRow(s);

    fireEvent.contextMenu(screen.getByText("personal"));
    expect(await screen.findByRole("menuitem", { name: "Open terminal here" })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Open console" })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Copy credentials as env" })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Copy profile command" })).toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Start" })).not.toBeInTheDocument();

    await user.click(screen.getByRole("menuitem", { name: "Stop" }));
    await waitFor(() => expect(stop).toHaveBeenCalledWith(s.id));
  });

  it("runs the credential actions from the context menu", async () => {
    const user = userEvent.setup();
    const s = activeIAM();
    const openConsole = vi.spyOn(api, "OpenConsole").mockResolvedValue();
    const env = vi.spyOn(api, "EnvText").mockResolvedValue("export X=1\n");
    renderRow(s);

    fireEvent.contextMenu(screen.getByText("personal"));
    await user.click(await screen.findByRole("menuitem", { name: "Open console" }));
    expect(openConsole).toHaveBeenCalledWith(s.id);

    fireEvent.contextMenu(screen.getByText("personal"));
    await user.click(await screen.findByRole("menuitem", { name: "Copy credentials as env" }));
    await waitFor(() => expect(env).toHaveBeenCalledWith(s.id));
    expect(await navigator.clipboard.readText()).toBe("export X=1\n");

    fireEvent.contextMenu(screen.getByText("personal"));
    await user.click(await screen.findByRole("menuitem", { name: "Copy profile command" }));
    await waitFor(async () => expect(await navigator.clipboard.readText()).toBe("aws --profile default"));
  });

  it("starts from the context menu", async () => {
    const user = userEvent.setup();
    const s = session({ name: "personal", kind: Kind.KindAWSIAMUser });
    const start = vi.spyOn(api, "Start").mockResolvedValue({} as never);
    renderRow(s);

    fireEvent.contextMenu(screen.getByText("personal"));
    await user.click(await screen.findByRole("menuitem", { name: "Start" }));
    await waitFor(() => expect(start).toHaveBeenCalledWith(s.id, ""));
  });

  it("removes a session from the row menu", async () => {
    const user = userEvent.setup();
    const s = session({ name: "personal", kind: Kind.KindAWSIAMUser });
    const remove = vi.spyOn(api, "RemoveSession").mockResolvedValue();
    renderRow(s);

    await user.click(screen.getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Remove" }));
    await waitFor(() => expect(remove).toHaveBeenCalledWith(s.id));
  });

  it("shows a remove failure as a toast", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "RemoveSession").mockRejectedValue(new Error("locked"));
    const error = vi.spyOn(toast, "error");
    renderRow(session({ name: "personal", kind: Kind.KindAWSIAMUser }));

    await user.click(screen.getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Remove" }));
    await waitFor(() => expect(error).toHaveBeenCalledWith("locked"));
  });

  it("omits the AWS-only items for a GCP session", async () => {
    const user = userEvent.setup();
    renderRow(session({ name: "data", kind: Kind.KindGCP, gcp: { projectId: "proj" } }));

    await user.click(screen.getByRole("button", { name: "More actions" }));
    expect(await screen.findByRole("menuitem", { name: "Rename" })).toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Set AWS profile name" })).not.toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Change region" })).not.toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Copy profile command" })).not.toBeInTheDocument();
  });

  it("renames a session from the row menu", async () => {
    const user = userEvent.setup();
    const s = session({ name: "personal", kind: Kind.KindAWSIAMUser });
    const rename = vi.spyOn(api, "RenameSession").mockResolvedValue();
    renderRow(s);

    await user.click(screen.getByRole("button", { name: "More actions" }));
    await user.click(await screen.findByRole("menuitem", { name: "Rename" }));
    const dialog = await screen.findByRole("dialog", { name: "Rename session" });
    const input = screen.getByDisplayValue("personal");
    await user.clear(input);
    await user.type(input, "work");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(rename).toHaveBeenCalledWith(s.id, "work"));
    await waitFor(() => expect(dialog).not.toBeInTheDocument());
  });

  it("sets the AWS profile name from the profile cell", async () => {
    const user = userEvent.setup();
    const s = session({ name: "personal", kind: Kind.KindAWSIAMUser });
    const setProfile = vi.spyOn(api, "SetProfile").mockResolvedValue();
    renderRow(s);

    await user.click(screen.getByRole("button", { name: "default" }));
    await screen.findByRole("dialog", { name: "AWS profile name" });
    await user.type(screen.getByPlaceholderText("default"), "work");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(setProfile).toHaveBeenCalledWith(s.id, "work"));
  });

  it("changes the region from the region cell", async () => {
    const user = userEvent.setup();
    const s = session({ name: "personal", kind: Kind.KindAWSIAMUser, region: "us-east-1" });
    const setRegion = vi.spyOn(api, "SetRegion").mockResolvedValue();
    renderRow(s);

    await user.click(screen.getByRole("button", { name: "us-east-1" }));
    await screen.findByRole("dialog", { name: "Change region" });
    await user.click(screen.getByRole("combobox"));
    await user.click(await screen.findByText("eu-west-1"));
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(setRegion).toHaveBeenCalledWith(s.id, "eu-west-1"));
  });
});
