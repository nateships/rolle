import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { describe, expect, it, vi } from "vitest";
import {
  AddAssumeRoleDialog,
  AddAzureDialog,
  AddGCPDialog,
  AddGCPImpersonationDialog,
  AddIAMUserDialog,
  AddSSODialog,
  LoginDialog,
  MFADialog,
} from "@/components/dialogs/Dialogs";
import { api, Cloud, Kind, type Session, type Workspace } from "@/lib/api";
import { celebrate } from "@/lib/celebrate";
import { integration, session } from "@/test/fixtures";

// Confetti draws on a canvas that jsdom does not have.
vi.mock("@/lib/celebrate", () => ({ celebrate: vi.fn() }));

const workspace = (o: Partial<Workspace> = {}) =>
  ({ version: 1, onboarded: true, sessions: [], integrations: [], ...o }) as unknown as Workspace;

/** Open a Radix select and choose one option by its label. */
async function pick(user: ReturnType<typeof userEvent.setup>, trigger: HTMLElement, option: string) {
  await user.click(trigger);
  await user.click(await screen.findByRole("option", { name: option }));
}

describe("AddSSODialog", () => {
  it("adds the portal and hands off to the login flow", async () => {
    const user = userEvent.setup();
    const integ = integration({ alias: "acme" });
    const add = vi.spyOn(api, "AddAWSSSO").mockResolvedValue(integ);
    const onLogin = vi.fn();
    render(<AddSSODialog open onClose={vi.fn()} onLogin={onLogin} />);

    const submit = screen.getByRole("button", { name: "Add and sign in" });
    await user.type(screen.getByPlaceholderText("acme"), "acme");
    // A start URL must use https.
    await user.type(screen.getByPlaceholderText("https://acme.awsapps.com/start"), "http://acme.awsapps.com/start");
    expect(submit).toBeDisabled();
    await user.clear(screen.getByPlaceholderText("https://acme.awsapps.com/start"));
    await user.type(screen.getByPlaceholderText("https://acme.awsapps.com/start"), "https://acme.awsapps.com/start");
    expect(submit).toBeEnabled();
    await user.click(submit);

    await waitFor(() => expect(onLogin).toHaveBeenCalledWith(integ));
    expect(add).toHaveBeenCalledWith("acme", "https://acme.awsapps.com/start", "us-east-1");
  });

  it("shows a backend error and stays open", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "AddAWSSSO").mockRejectedValue(new Error("alias taken"));
    const error = vi.spyOn(toast, "error");
    const onLogin = vi.fn();
    render(<AddSSODialog open onClose={vi.fn()} onLogin={onLogin} />);

    await user.type(screen.getByPlaceholderText("acme"), "acme");
    await user.type(screen.getByPlaceholderText("https://acme.awsapps.com/start"), "https://acme.awsapps.com/start");
    await user.click(screen.getByRole("button", { name: "Add and sign in" }));

    await waitFor(() => expect(error).toHaveBeenCalledWith("alias taken"));
    expect(onLogin).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog", { name: "Add Identity Center portal" })).toBeInTheDocument();
  });
});

describe("LoginDialog", () => {
  const acme = integration({ id: "acme", alias: "acme", cloud: Cloud.CloudAWS });

  it("shows the device code and lets the user reopen the page or cancel", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "StartSSOLogin").mockResolvedValue({ verificationUri: "https://verify", userCode: "ABCD-1234" });
    // Approval never arrives in this test.
    vi.spyOn(api, "WaitSSOLogin").mockReturnValue(new Promise(() => {}) as never);
    const open = vi.spyOn(api, "OpenURL").mockResolvedValue();
    const cancel = vi.spyOn(api, "CancelSSOLogin").mockResolvedValue();
    const onClose = vi.fn();
    render(<LoginDialog integration={acme} onClose={onClose} />);

    expect(screen.getByRole("dialog", { name: "Sign in to acme" })).toBeInTheDocument();
    expect(await screen.findByText("ABCD-1234")).toBeInTheDocument();
    expect(screen.getByText("Approve the request in your browser and confirm this code.")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /reopen the page/i }));
    expect(open).toHaveBeenCalledWith("https://verify");

    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(cancel).toHaveBeenCalledWith("acme");
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("reports the discovered sessions once approval arrives", async () => {
    vi.spyOn(api, "StartSSOLogin").mockResolvedValue({ verificationUri: "https://verify", userCode: "" });
    const found = [session({ kind: Kind.KindAWSSSORole }), session({ kind: Kind.KindAWSSSORole })];
    vi.spyOn(api, "WaitSSOLogin").mockResolvedValue(found);
    const success = vi.spyOn(toast, "success");
    const onClose = vi.fn();
    const onDone = vi.fn();
    render(<LoginDialog integration={acme} onClose={onClose} onDone={onDone} />);

    await waitFor(() => expect(onDone).toHaveBeenCalledWith(acme, found));
    expect(success).toHaveBeenCalledWith("Signed in to acme", { description: "2 new sessions discovered." });
    expect(celebrate).toHaveBeenCalledWith("small");
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("closes with an error toast when the login cannot start", async () => {
    vi.spyOn(api, "StartSSOLogin").mockRejectedValue(new Error("network down"));
    const error = vi.spyOn(toast, "error");
    const onClose = vi.fn();
    render(<LoginDialog integration={acme} onClose={onClose} />);

    await waitFor(() => expect(error).toHaveBeenCalledWith("network down"));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("runs the Azure sign-in without a device code", async () => {
    const user = userEvent.setup();
    const contoso = integration({ id: "contoso", alias: "contoso", cloud: Cloud.CloudAzure });
    let finish: (s: Session[]) => void = () => {};
    vi.spyOn(api, "AzureLogin").mockReturnValue(new Promise<Session[]>((r) => (finish = r)) as never);
    const cancel = vi.spyOn(api, "CancelSSOLogin").mockResolvedValue();
    const success = vi.spyOn(toast, "success");
    const onClose = vi.fn();
    render(<LoginDialog integration={contoso} onClose={onClose} />);

    expect(screen.getByText("Finish signing in with Microsoft in your browser.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Cancel" })).not.toBeInTheDocument();

    // Escape closes the dialog; Azure has nothing to cancel on the backend.
    await user.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(cancel).not.toHaveBeenCalled();

    // The sign-in still finishes in the background and reports its outcome.
    finish([]);
    await waitFor(() =>
      expect(success).toHaveBeenCalledWith("Signed in to contoso", { description: "No new sessions." }),
    );
  });

  it("renders nothing without an integration", () => {
    render(<LoginDialog integration={null} onClose={vi.fn()} />);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
});

describe("AddAssumeRoleDialog", () => {
  const base = session({ name: "base", kind: Kind.KindAWSIAMUser });
  const gcp = session({ name: "data", kind: Kind.KindGCP, gcp: { projectId: "p" } });
  const ws = workspace({ sessions: [base, gcp] });

  it("requires a name, a valid role ARN, and an AWS source session", async () => {
    const user = userEvent.setup();
    const add = vi.spyOn(api, "AddAssumeRole").mockResolvedValue(session({}));
    const success = vi.spyOn(toast, "success");
    const onClose = vi.fn();
    render(<AddAssumeRoleDialog open onClose={onClose} workspace={ws} />);

    const submit = screen.getByRole("button", { name: "Add session" });
    await user.type(screen.getByPlaceholderText("prod-admin"), "prod-admin");
    const arn = screen.getByPlaceholderText("arn:aws:iam::123456789012:role/Admin");
    await user.type(arn, "arn:aws:iam::123:role/Admin");
    expect(submit).toBeDisabled();
    await user.clear(arn);
    await user.type(arn, "arn:aws:iam::123456789012:role/Admin");
    // Still no source session.
    expect(submit).toBeDisabled();

    await user.click(screen.getByRole("combobox", { name: "Source session" }));
    // Only AWS sessions can be the source.
    expect(screen.queryByRole("option", { name: "data" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("option", { name: "base" }));
    expect(submit).toBeEnabled();
    await user.click(submit);

    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1));
    expect(add).toHaveBeenCalledWith({
      name: "prod-admin",
      roleArn: "arn:aws:iam::123456789012:role/Admin",
      region: "us-east-1",
      sourceRef: base.id,
      externalId: "",
      profile: "",
    });
    expect(success).toHaveBeenCalledWith("prod-admin added");
  });

  it("shows a backend error and stays open", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "AddAssumeRole").mockRejectedValue(new Error("duplicate name"));
    const error = vi.spyOn(toast, "error");
    const onClose = vi.fn();
    render(<AddAssumeRoleDialog open onClose={onClose} workspace={ws} />);

    await user.type(screen.getByPlaceholderText("prod-admin"), "prod-admin");
    await user.type(
      screen.getByPlaceholderText("arn:aws:iam::123456789012:role/Admin"),
      "arn:aws:iam::123456789012:role/Admin",
    );
    await pick(user, screen.getByRole("combobox", { name: "Source session" }), "base");
    await user.click(screen.getByRole("button", { name: "Add session" }));

    await waitFor(() => expect(error).toHaveBeenCalledWith("duplicate name"));
    expect(onClose).not.toHaveBeenCalled();
  });
});

describe("AddIAMUserDialog", () => {
  it("adds the user with the default region", async () => {
    const user = userEvent.setup();
    const add = vi.spyOn(api, "AddIAMUser").mockResolvedValue(session({}));
    const success = vi.spyOn(toast, "success");
    const onClose = vi.fn();
    render(<AddIAMUserDialog open onClose={onClose} defaultRegion="eu-west-1" />);

    const submit = screen.getByRole("button", { name: "Add session" });
    await user.type(screen.getByPlaceholderText("personal"), "personal");
    await user.type(screen.getByPlaceholderText("AKIA…"), "AKIA123");
    expect(submit).toBeDisabled();
    // The secret field follows the access key in tab order.
    await user.tab();
    await user.keyboard("shh");
    expect(submit).toBeEnabled();
    await user.click(submit);

    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1));
    expect(add).toHaveBeenCalledWith({
      name: "personal",
      region: "eu-west-1",
      accessKeyId: "AKIA123",
      secretAccessKey: "shh",
      mfaDevice: "",
    });
    expect(success).toHaveBeenCalledWith("personal added");
  });

  it("shows a backend error and stays open", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "AddIAMUser").mockRejectedValue(new Error("keychain locked"));
    const error = vi.spyOn(toast, "error");
    const onClose = vi.fn();
    render(<AddIAMUserDialog open onClose={onClose} />);

    await user.type(screen.getByPlaceholderText("personal"), "personal");
    await user.type(screen.getByPlaceholderText("AKIA…"), "AKIA123");
    await user.tab();
    await user.keyboard("shh");
    await user.click(screen.getByRole("button", { name: "Add session" }));

    await waitFor(() => expect(error).toHaveBeenCalledWith("keychain locked"));
    expect(onClose).not.toHaveBeenCalled();
  });
});

describe("MFADialog", () => {
  it("accepts six to eight digits and submits the code", async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();
    render(<MFADialog open onClose={vi.fn()} onSubmit={onSubmit} />);

    const input = screen.getByPlaceholderText("000000");
    const submit = screen.getByRole("button", { name: "Start session" });
    // Letters are dropped.
    await user.type(input, "12ab34");
    expect(input).toHaveValue("1234");
    expect(submit).toBeDisabled();
    await user.type(input, "56");
    expect(submit).toBeEnabled();
    // The code caps at eight digits.
    await user.type(input, "789");
    expect(input).toHaveValue("12345678");
    await user.keyboard("{Enter}");
    expect(onSubmit).toHaveBeenCalledWith("12345678");
  });

  it("closes on Escape", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<MFADialog open onClose={onClose} onSubmit={vi.fn()} />);
    await user.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});

describe("AddAzureDialog", () => {
  it("adds the tenant and hands off to the login flow", async () => {
    const user = userEvent.setup();
    const integ = integration({ alias: "contoso", cloud: Cloud.CloudAzure });
    const add = vi.spyOn(api, "AddAzure").mockResolvedValue(integ);
    const onLogin = vi.fn();
    render(<AddAzureDialog open onClose={vi.fn()} onLogin={onLogin} />);

    const submit = screen.getByRole("button", { name: "Add and sign in" });
    expect(submit).toBeDisabled();
    await user.type(screen.getByPlaceholderText("contoso"), "contoso");
    await user.type(screen.getByPlaceholderText("contoso.onmicrosoft.com"), "contoso.onmicrosoft.com");
    await user.click(submit);

    await waitFor(() => expect(onLogin).toHaveBeenCalledWith(integ));
    expect(add).toHaveBeenCalledWith("contoso", "contoso.onmicrosoft.com");
  });

  it("shows a backend error", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "AddAzure").mockRejectedValue(new Error("bad tenant"));
    const error = vi.spyOn(toast, "error");
    render(<AddAzureDialog open onClose={vi.fn()} onLogin={vi.fn()} />);

    await user.type(screen.getByPlaceholderText("contoso"), "contoso");
    await user.click(screen.getByRole("button", { name: "Add and sign in" }));
    await waitFor(() => expect(error).toHaveBeenCalledWith("bad tenant"));
  });
});

describe("AddGCPDialog", () => {
  const ready = {
    ready: true,
    account: "nate@example.com",
    loginCommand: "gcloud auth application-default login",
    gcloudFound: true,
    gcloudPath: "/usr/bin/gcloud",
    installUrl: "https://cloud.google.com/sdk",
  };

  it("discovers projects for the signed-in gcloud account", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "GCPStatus").mockResolvedValue(ready);
    const add = vi.spyOn(api, "AddGCP").mockResolvedValue([session({ kind: Kind.KindGCP })]);
    const success = vi.spyOn(toast, "success");
    const onClose = vi.fn();
    render(<AddGCPDialog open onClose={onClose} />);

    expect(await screen.findByText(/signed in as/i)).toHaveTextContent("nate@example.com");
    const alias = screen.getByDisplayValue("gcp");
    await user.clear(alias);
    await user.type(alias, "work");
    await user.click(screen.getByRole("button", { name: "Discover projects" }));

    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1));
    expect(add).toHaveBeenCalledWith("work");
    expect(success).toHaveBeenCalledWith("1 project discovered");
    expect(celebrate).toHaveBeenCalledWith("small");
  });

  it("treats a null project list as zero projects", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "GCPStatus").mockResolvedValue(ready);
    vi.spyOn(api, "AddGCP").mockResolvedValue(null);
    const success = vi.spyOn(toast, "success");
    render(<AddGCPDialog open onClose={vi.fn()} />);

    await user.click(await screen.findByRole("button", { name: "Discover projects" }));
    await waitFor(() => expect(success).toHaveBeenCalledWith("0 projects discovered"));
  });

  it("shows a backend error", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "GCPStatus").mockResolvedValue(ready);
    vi.spyOn(api, "AddGCP").mockRejectedValue(new Error("no permission"));
    const error = vi.spyOn(toast, "error");
    const onClose = vi.fn();
    render(<AddGCPDialog open onClose={onClose} />);

    await user.click(await screen.findByRole("button", { name: "Discover projects" }));
    await waitFor(() => expect(error).toHaveBeenCalledWith("no permission"));
    expect(onClose).not.toHaveBeenCalled();
  });
});

describe("AddGCPImpersonationDialog", () => {
  const g1 = integration({ id: "g1", alias: "work", cloud: Cloud.CloudGCP, gcp: { account: "a@x.com" } });
  const g2 = integration({ id: "g2", alias: "home", cloud: Cloud.CloudGCP, gcp: { account: "b@x.com" } });
  const projects = [
    session({ kind: Kind.KindGCP, integrationId: "g1", gcp: { projectId: "proj-b" } }),
    session({ kind: Kind.KindGCP, integrationId: "g1", gcp: { projectId: "proj-a" } }),
  ];

  it("picks a known project and validates the service account email", async () => {
    const user = userEvent.setup();
    const add = vi.spyOn(api, "AddGCPImpersonation").mockResolvedValue(session({}));
    const success = vi.spyOn(toast, "success");
    const onClose = vi.fn();
    render(
      <AddGCPImpersonationDialog
        open
        onClose={onClose}
        workspace={workspace({ integrations: [g1], sessions: projects })}
      />,
    );

    // One account needs no account picker.
    expect(screen.getAllByRole("combobox")).toHaveLength(1);
    const submit = screen.getByRole("button", { name: "Add session" });
    await user.type(screen.getByPlaceholderText("prod-deployer"), "deployer");
    await pick(user, screen.getByRole("combobox", { name: "Project ID" }), "proj-a");
    const email = screen.getByPlaceholderText("deployer@my-project.iam.gserviceaccount.com");
    await user.type(email, "deployer@proj-a");
    expect(submit).toBeDisabled();
    await user.type(email, ".iam.gserviceaccount.com");
    expect(submit).toBeEnabled();
    await user.click(submit);

    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1));
    expect(add).toHaveBeenCalledWith({
      name: "deployer",
      integrationRef: "g1",
      projectId: "proj-a",
      serviceAccount: "deployer@proj-a.iam.gserviceaccount.com",
    });
    expect(success).toHaveBeenCalledWith("deployer added");
  });

  it("takes a typed project ID when no project is known, and lets the user choose the account", async () => {
    const user = userEvent.setup();
    const add = vi.spyOn(api, "AddGCPImpersonation").mockResolvedValue(session({}));
    render(<AddGCPImpersonationDialog open onClose={vi.fn()} workspace={workspace({ integrations: [g1, g2] })} />);

    await user.type(screen.getByPlaceholderText("prod-deployer"), "deployer");
    await pick(user, screen.getByRole("combobox", { name: "Account" }), "home");
    await user.type(screen.getByPlaceholderText("my-project"), "typed-proj");
    await user.type(
      screen.getByPlaceholderText("deployer@my-project.iam.gserviceaccount.com"),
      "sa@typed-proj.iam.gserviceaccount.com",
    );
    await user.click(screen.getByRole("button", { name: "Add session" }));

    await waitFor(() =>
      expect(add).toHaveBeenCalledWith({
        name: "deployer",
        integrationRef: "g2",
        projectId: "typed-proj",
        serviceAccount: "sa@typed-proj.iam.gserviceaccount.com",
      }),
    );
  });

  it("shows a backend error and stays open", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "AddGCPImpersonation").mockRejectedValue(new Error("token creator role missing"));
    const error = vi.spyOn(toast, "error");
    const onClose = vi.fn();
    render(<AddGCPImpersonationDialog open onClose={onClose} workspace={workspace({ integrations: [g1] })} />);

    await user.type(screen.getByPlaceholderText("prod-deployer"), "deployer");
    await user.type(screen.getByPlaceholderText("my-project"), "p");
    await user.type(
      screen.getByPlaceholderText("deployer@my-project.iam.gserviceaccount.com"),
      "sa@p.iam.gserviceaccount.com",
    );
    await user.click(screen.getByRole("button", { name: "Add session" }));

    await waitFor(() => expect(error).toHaveBeenCalledWith("token creator role missing"));
    expect(onClose).not.toHaveBeenCalled();
  });
});
