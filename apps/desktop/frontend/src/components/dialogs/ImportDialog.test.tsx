import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ImportDialog } from "@/components/dialogs/ImportDialog";
import { api, Cloud, Kind, type Workspace } from "@/lib/api";
import { celebrate } from "@/lib/celebrate";
import { integration, session } from "@/test/fixtures";

// Confetti draws on a canvas that jsdom does not have.
vi.mock("@/lib/celebrate", () => ({ celebrate: vi.fn() }));

const engineering = {
  alias: "Engineering",
  startUrl: "https://engineering.awsapps.com/start",
  region: "us-east-1",
  profiles: ["AWS_Dev", "AWS_Prod"],
  hasToken: true,
  source: "aws-cli",
};
const globex = {
  alias: "globex",
  startUrl: "https://globex.awsapps.com/start",
  region: "us-west-2",
  profiles: [],
  hasToken: false,
  source: "granted",
};
const tenant = { tenantId: "t-1", account: "nate@contoso.com", source: "az" };
const found = {
  awsPortals: [engineering, globex],
  azureTenants: [tenant],
  gcp: { account: "nate@example.com" },
  leapp: { iamUsers: [{ name: "personal-old" }], chainedRoles: [{ name: "prod-admin-old" }], ssoRoles: 28 },
};
const empty = { awsPortals: [], azureTenants: [], gcp: null, leapp: null };

const workspace = (o: Partial<Workspace> = {}) =>
  ({ version: 1, onboarded: true, sessions: [], integrations: [], ...o }) as unknown as Workspace;

/** The Import button in the row titled `title`. */
async function importButton(title: string) {
  const row = (await screen.findByText(title)).closest("li")!;
  return within(row).getByRole("button", { name: /import/i });
}

describe("ImportDialog", () => {
  let discover: ReturnType<typeof vi.spyOn>;
  beforeEach(() => {
    discover = vi.spyOn(api, "Discover").mockResolvedValue(found as never);
  });

  function renderDialog(ws = workspace(), props: { onClose?: () => void; onLogin?: () => void } = {}) {
    const onClose = props.onClose ?? vi.fn();
    const onLogin = props.onLogin ?? vi.fn();
    render(<ImportDialog open onClose={onClose} workspace={ws} onLogin={onLogin} />);
    return { onClose, onLogin };
  }

  it("scans once and lists everything the CLIs know about", async () => {
    renderDialog();
    expect(screen.getByText("Scanning…")).toBeInTheDocument();
    expect(await screen.findByText("Engineering")).toBeInTheDocument();
    expect(screen.getByText("globex")).toBeInTheDocument();
    expect(screen.getByText("nate@contoso.com")).toBeInTheDocument();
    expect(screen.getByText("nate@example.com")).toBeInTheDocument();
    expect(screen.getByText("2 Leapp sessions")).toBeInTheDocument();
    expect(discover).toHaveBeenCalledTimes(1);
  });

  it("does not scan while closed", () => {
    render(<ImportDialog open={false} onClose={vi.fn()} workspace={workspace()} onLogin={vi.fn()} />);
    expect(discover).not.toHaveBeenCalled();
  });

  it("imports a signed-in AWS portal with its roles", async () => {
    const user = userEvent.setup();
    const roles = [session({ kind: Kind.KindAWSSSORole }), session({ kind: Kind.KindAWSSSORole })];
    const imp = vi
      .spyOn(api, "ImportAWSSSO")
      .mockResolvedValue({ integration: integration({}), loggedIn: true, sessions: roles });
    const success = vi.spyOn(toast, "success");
    const { onLogin } = renderDialog();

    await user.click(await importButton("Engineering"));

    await waitFor(() =>
      expect(success).toHaveBeenCalledWith("Imported Engineering", {
        description: "2 roles from your AWS CLI sign-in.",
      }),
    );
    expect(imp).toHaveBeenCalledWith("Engineering", "https://engineering.awsapps.com/start", "us-east-1");
    expect(celebrate).toHaveBeenCalledWith("small");
    expect(onLogin).not.toHaveBeenCalled();
  });

  it("hands a signed-out AWS portal to the login flow", async () => {
    const user = userEvent.setup();
    const integ = integration({ alias: "globex" });
    vi.spyOn(api, "ImportAWSSSO").mockResolvedValue({ integration: integ, loggedIn: false, sessions: [] });
    const { onClose, onLogin } = renderDialog();

    await user.click(await importButton("globex"));

    await waitFor(() => expect(onLogin).toHaveBeenCalledWith(integ));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("adds an Azure tenant under its domain and hands it to the login flow", async () => {
    const user = userEvent.setup();
    const integ = integration({ alias: "contoso", cloud: Cloud.CloudAzure });
    const add = vi.spyOn(api, "AddAzure").mockResolvedValue(integ);
    const { onClose, onLogin } = renderDialog();

    await user.click(await importButton("nate@contoso.com"));

    await waitFor(() => expect(onLogin).toHaveBeenCalledWith(integ));
    expect(add).toHaveBeenCalledWith("contoso", "t-1");
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("imports the gcloud account and reports the project count", async () => {
    const user = userEvent.setup();
    const add = vi.spyOn(api, "AddGCP").mockResolvedValue([session({ kind: Kind.KindGCP })]);
    const success = vi.spyOn(toast, "success");
    renderDialog();

    await user.click(await importButton("nate@example.com"));

    await waitFor(() => expect(success).toHaveBeenCalledWith("1 project discovered"));
    expect(add).toHaveBeenCalledWith("gcp");
  });

  it("imports Leapp sessions, names the skipped ones, and rescans", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "ImportLeappSessions").mockResolvedValue({
      sessions: [session({}), session({})],
      skipped: ["broken-one", "broken-two"],
    });
    const success = vi.spyOn(toast, "success");
    renderDialog();

    await user.click(await importButton("2 Leapp sessions"));

    await waitFor(() =>
      expect(success).toHaveBeenCalledWith("Imported 2 sessions from Leapp", {
        description: "2 skipped: broken-one…",
      }),
    );
    await waitFor(() => expect(discover).toHaveBeenCalledTimes(2));
  });

  it("shows an import failure and re-enables the buttons", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "ImportAWSSSO").mockRejectedValue(new Error("token expired"));
    const error = vi.spyOn(toast, "error");
    renderDialog();

    const button = await importButton("Engineering");
    await user.click(button);

    await waitFor(() => expect(error).toHaveBeenCalledWith("token expired"));
    expect(button).toBeEnabled();
  });

  it("offers a rescan when nothing new is found", async () => {
    const user = userEvent.setup();
    discover.mockResolvedValue(empty as never);
    renderDialog();

    expect(await screen.findByText(/nothing new found/i)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /scan again/i }));
    await waitFor(() => expect(discover).toHaveBeenCalledTimes(2));
  });

  it("treats a failed scan as an empty result", async () => {
    discover.mockRejectedValue(new Error("scanner crashed"));
    renderDialog();
    expect(await screen.findByText(/nothing new found/i)).toBeInTheDocument();
  });

  it("hides identities the workspace already has", async () => {
    // A trailing slash on the stored start URL still matches.
    const ws = workspace({
      integrations: [
        integration({
          cloud: Cloud.CloudAWS,
          awsSso: { startUrl: "https://engineering.awsapps.com/start/", region: "x" },
        }),
        integration({ cloud: Cloud.CloudAzure, azure: { tenantId: "t-1" } }),
        integration({ cloud: Cloud.CloudGCP, gcp: { account: "nate@example.com" } }),
      ],
      sessions: [session({ name: "personal-old" }), session({ name: "prod-admin-old" })],
    });
    renderDialog(ws);

    expect(await screen.findByText("globex")).toBeInTheDocument();
    expect(screen.queryByText("Engineering")).not.toBeInTheDocument();
    expect(screen.queryByText("nate@contoso.com")).not.toBeInTheDocument();
    expect(screen.queryByText("nate@example.com")).not.toBeInTheDocument();
    expect(screen.queryByText(/leapp sessions/i)).not.toBeInTheDocument();
  });
});
