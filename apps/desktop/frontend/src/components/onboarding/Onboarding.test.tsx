import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@/components/ui/tooltip";
import { Onboarding } from "@/components/onboarding/Onboarding";
import { api, type Integration, type Session, type Workspace } from "@/lib/api";

const empty = { version: 1, onboarded: false, sessions: [], integrations: [] } as unknown as Workspace;

const ssoRole = (name: string, extra: Record<string, unknown> = {}) =>
  ({
    id: name,
    name,
    kind: "aws-sso-role",
    region: "us-east-1",
    status: "inactive",
    aws: { accountId: "123456789012", roleName: name.split("/")[1] },
    ...extra,
  }) as unknown as Session;
const azureSub = {
  id: "az-sub",
  name: "Contoso Production",
  kind: "azure",
  status: "inactive",
  azure: { subscriptionId: "0f1e2d3c", tenantId: "t-1" },
} as unknown as Session;
const gcpProject = {
  id: "gcp-proj",
  name: "data-platform",
  kind: "gcp",
  status: "inactive",
  gcp: { projectId: "data-platform-4821" },
} as unknown as Session;
const acme = { id: "acme", alias: "acme", cloud: "aws", awsSso: { startUrl: "https://acme.awsapps.com/start" } };

const noFinds = { awsPortals: [], azureTenants: [], gcp: null };
const gcpReady = {
  ready: true,
  account: "nate@example.com",
  loginCommand: "gcloud auth application-default login",
  gcloudFound: true,
  gcloudPath: "/opt/homebrew/bin/gcloud",
  installUrl: "https://cloud.google.com/sdk/docs/install",
};
const gcpNotReady = { ...gcpReady, ready: false, account: "" };

/** A promise the test settles by hand, so a step can be observed while a call is pending. */
function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function renderOnboarding(workspace: Workspace = empty) {
  return render(
    <TooltipProvider>
      <Onboarding workspace={workspace} />
    </TooltipProvider>,
  );
}

/** Wait for the step whose title is `name`. The previous title may still be on screen. */
const heading = (name: string) => screen.findByRole("heading", { level: 2, name });
const at = (query: string) => window.history.replaceState({}, "", `/${query}`);

/** Fill the Identity Center form and submit it. */
async function submitSSO(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByPlaceholderText("acme"), "acme");
  await user.type(screen.getByPlaceholderText("https://acme.awsapps.com/start"), "https://acme.awsapps.com/start");
  await user.click(screen.getByRole("button", { name: /^sign in$/i }));
}

/** Spy the AWS device flow so the approve step waits until the test resolves it. */
function stubAWSLogin() {
  const wait = deferred<Session[]>();
  vi.spyOn(api, "AddAWSSSO").mockResolvedValue({ id: "i1", alias: "acme", cloud: "aws" } as Integration);
  vi.spyOn(api, "StartSSOLogin").mockResolvedValue({
    verificationUri: "https://device.example/verify",
    userCode: "ABCD-1234",
  });
  vi.spyOn(api, "WaitSSOLogin").mockReturnValue(wait.promise as never);
  const cancel = vi.spyOn(api, "CancelSSOLogin").mockResolvedValue();
  return { wait, cancel };
}

describe("Onboarding", () => {
  beforeEach(() => {
    at("");
    // The cloud step scans the machine; keep the test offline and instant.
    vi.spyOn(api, "Discover").mockResolvedValue(noFinds as never);
  });

  it("renders the welcome step", () => {
    renderOnboarding();
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent("Assume any role, any cloud.");
    expect(screen.getByRole("button", { name: /get started/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Skip" })).toBeEnabled();
  });

  it("advances to the cloud step on the primary button", async () => {
    const user = userEvent.setup();
    renderOnboarding();
    await user.click(screen.getByRole("button", { name: /get started/i }));
    expect(await heading("Where do your roles live?")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /get started/i })).not.toBeInTheDocument();
    expect(history.state).toMatchObject({ step: "cloud" });
  });

  it("jumps to a step named in the query string", () => {
    at("?step=cloud");
    renderOnboarding();
    expect(screen.getByRole("heading", { level: 2 })).toHaveTextContent("Where do your roles live?");
  });

  it("offers the rolle command on the final step", async () => {
    at("?step=done");
    renderOnboarding();
    expect(await screen.findByRole("button", { name: /install command/i })).toBeInTheDocument();
  });

  it("skips to the final step without a cloud", async () => {
    const user = userEvent.setup();
    renderOnboarding();
    await user.click(screen.getByRole("button", { name: "Skip" }));
    expect(await heading("You're all set.")).toBeInTheDocument();
    expect(await screen.findByRole("button", { name: /install command/i })).toBeInTheDocument();
  });

  it("counts the workspace sessions in the footer", () => {
    renderOnboarding({ ...empty, sessions: [ssoRole("Acme Prod/Admin")] });
    expect(screen.getByText("1 session(s) in your workspace")).toBeInTheDocument();
  });

  it("marks a cloud as connected when the workspace has an integration for it", () => {
    at("?step=cloud");
    renderOnboarding({ ...empty, integrations: [acme as unknown as Integration] });
    const card = screen.getByRole("button", { name: /amazon web services/i });
    expect(within(card).getByText("Connected")).toBeInTheDocument();
    expect(screen.getAllByText("Connected")).toHaveLength(1);
  });

  it("opens the connect step for the chosen cloud", async () => {
    const user = userEvent.setup();
    at("?step=cloud");
    renderOnboarding();
    await user.click(screen.getByRole("button", { name: /microsoft azure/i }));
    expect(await heading("Connect to Azure")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /choose a different cloud/i }));
    expect(await heading("Where do your roles live?")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /google cloud/i }));
    expect(await heading("Connect to Google Cloud")).toBeInTheDocument();
  });

  describe("AWS", () => {
    beforeEach(() => at("?step=connect&cloud=aws"));

    it("signs in to Identity Center through the device flow", async () => {
      const user = userEvent.setup();
      const { wait } = stubAWSLogin();
      const open = vi.spyOn(api, "OpenURL").mockResolvedValue();
      renderOnboarding();
      expect(screen.getByRole("heading", { level: 2 })).toHaveTextContent("Connect to AWS");
      expect(screen.getByRole("button", { name: /^sign in$/i })).toBeDisabled();
      await submitSSO(user);
      expect(await heading("Approve in your browser")).toBeInTheDocument();
      expect(api.AddAWSSSO).toHaveBeenCalledWith("acme", "https://acme.awsapps.com/start", "us-east-1");
      expect(api.StartSSOLogin).toHaveBeenCalledWith("i1");
      expect(screen.getByText("ABCD-1234")).toBeInTheDocument();
      expect(screen.getByText(/enter this code/i)).toBeInTheDocument();
      await user.click(screen.getByRole("button", { name: /reopen the page/i }));
      expect(open).toHaveBeenCalledWith("https://device.example/verify");
      wait.resolve([ssoRole("Acme Prod/AdministratorAccess"), ssoRole("Acme Dev/PowerUser")]);
      expect(await heading("2 sessions ready")).toBeInTheDocument();
      expect(api.WaitSSOLogin).toHaveBeenCalledWith("i1");
      expect(screen.getByText("Acme Prod/AdministratorAccess")).toBeInTheDocument();
      expect(screen.getByText("Acme Dev/PowerUser")).toBeInTheDocument();
    });

    it("cancels a pending sign-in and returns to the form", async () => {
      const user = userEvent.setup();
      const { wait, cancel } = stubAWSLogin();
      const error = vi.spyOn(toast, "error");
      renderOnboarding();
      await submitSSO(user);
      expect(await heading("Approve in your browser")).toBeInTheDocument();
      await user.click(screen.getByRole("button", { name: "Cancel" }));
      expect(cancel).toHaveBeenCalledWith("i1");
      expect(await heading("Connect to AWS")).toBeInTheDocument();
      // The backend rejects the abandoned wait. A cancel is not an error.
      await act(async () => wait.reject(new Error("cancelled")));
      expect(error).not.toHaveBeenCalled();
      expect(screen.getByRole("heading", { level: 2 })).toHaveTextContent("Connect to AWS");
    });

    it("reports a failed portal registration and stays on the form", async () => {
      const user = userEvent.setup();
      vi.spyOn(api, "AddAWSSSO").mockRejectedValue(new Error("portal unreachable"));
      const error = vi.spyOn(toast, "error");
      renderOnboarding();
      await submitSSO(user);
      await waitFor(() => expect(error).toHaveBeenCalledWith("portal unreachable"));
      expect(screen.getByRole("heading", { level: 2 })).toHaveTextContent("Connect to AWS");
      expect(screen.getByRole("button", { name: /^sign in$/i })).toBeEnabled();
    });

    it("returns to the form when the approval fails", async () => {
      const user = userEvent.setup();
      const { wait } = stubAWSLogin();
      const error = vi.spyOn(toast, "error");
      renderOnboarding();
      await submitSSO(user);
      expect(await heading("Approve in your browser")).toBeInTheDocument();
      wait.reject(new Error("authorization expired"));
      expect(await heading("Connect to AWS")).toBeInTheDocument();
      expect(error).toHaveBeenCalledWith("authorization expired");
    });

    it("adds an IAM user from an access key", async () => {
      const user = userEvent.setup();
      const add = vi.spyOn(api, "AddIAMUser").mockResolvedValue(ssoRole("personal", { kind: "aws-iam-user", aws: {} }));
      renderOnboarding();
      await user.click(screen.getByRole("button", { name: /access key/i }));
      const submit = screen.getByRole("button", { name: /add session/i });
      expect(submit).toBeDisabled();
      await user.type(screen.getByPlaceholderText("personal"), "personal");
      await user.type(screen.getByPlaceholderText("AKIA…"), "AKIAEXAMPLE");
      await user.type(screen.getByLabelText("Secret access key"), "s3cret");
      expect(submit).toBeEnabled();
      await user.click(submit);
      expect(await heading("One session ready")).toBeInTheDocument();
      expect(add).toHaveBeenCalledWith({
        name: "personal",
        region: "us-east-1",
        accessKeyId: "AKIAEXAMPLE",
        secretAccessKey: "s3cret",
        mfaDevice: "",
      });
      expect(screen.getByText("personal")).toBeInTheDocument();
    });

    it("reports a rejected access key", async () => {
      const user = userEvent.setup();
      vi.spyOn(api, "AddIAMUser").mockRejectedValue(new Error("bad key"));
      const error = vi.spyOn(toast, "error");
      renderOnboarding();
      await user.click(screen.getByRole("button", { name: /access key/i }));
      await user.type(screen.getByPlaceholderText("personal"), "personal");
      await user.type(screen.getByPlaceholderText("AKIA…"), "AKIAEXAMPLE");
      await user.type(screen.getByLabelText("Secret access key"), "s3cret");
      await user.click(screen.getByRole("button", { name: /add session/i }));
      await waitFor(() => expect(error).toHaveBeenCalledWith("bad key"));
      expect(screen.getByRole("heading", { level: 2 })).toHaveTextContent("Connect to AWS");
    });
  });

  describe("Azure", () => {
    beforeEach(() => at("?step=connect&cloud=azure"));

    it("signs in with Microsoft and lists the subscriptions", async () => {
      const user = userEvent.setup();
      const login = deferred<Session[]>();
      vi.spyOn(api, "AddAzure").mockResolvedValue({ id: "az1", alias: "contoso", cloud: "azure" } as Integration);
      vi.spyOn(api, "AzureLogin").mockReturnValue(login.promise as never);
      renderOnboarding();
      const submit = screen.getByRole("button", { name: /sign in with microsoft/i });
      expect(submit).toBeDisabled();
      await user.type(screen.getByPlaceholderText("contoso"), "contoso");
      await user.type(screen.getByPlaceholderText("contoso.onmicrosoft.com"), "contoso.onmicrosoft.com");
      await user.click(submit);
      expect(await heading("Sign in with Microsoft")).toBeInTheDocument();
      expect(screen.getByText(/a browser window is open for contoso/i)).toBeInTheDocument();
      // A Microsoft sign-in has no device code and no cancel.
      expect(screen.queryByRole("button", { name: "Cancel" })).not.toBeInTheDocument();
      expect(api.AddAzure).toHaveBeenCalledWith("contoso", "contoso.onmicrosoft.com");
      login.resolve([azureSub]);
      expect(await heading("One session ready")).toBeInTheDocument();
      expect(api.AzureLogin).toHaveBeenCalledWith("az1");
      expect(screen.getByText("Contoso Production")).toBeInTheDocument();
    });

    it("returns to the form when the sign-in fails", async () => {
      const user = userEvent.setup();
      vi.spyOn(api, "AddAzure").mockResolvedValue({ id: "az1", alias: "contoso", cloud: "azure" } as Integration);
      vi.spyOn(api, "AzureLogin").mockRejectedValue(new Error("tenant refused"));
      const error = vi.spyOn(toast, "error");
      renderOnboarding();
      await user.type(screen.getByPlaceholderText("contoso"), "contoso");
      await user.click(screen.getByRole("button", { name: /sign in with microsoft/i }));
      await waitFor(() => expect(error).toHaveBeenCalledWith("tenant refused"));
      expect(await heading("Connect to Azure")).toBeInTheDocument();
    });
  });

  describe("Google Cloud", () => {
    beforeEach(() => at("?step=connect&cloud=gcp"));

    it("discovers projects once gcloud credentials exist", async () => {
      const user = userEvent.setup();
      vi.spyOn(api, "GCPStatus").mockResolvedValue(gcpReady);
      const add = vi.spyOn(api, "AddGCP").mockResolvedValue([gcpProject]);
      renderOnboarding();
      expect(screen.getByText(/looking for gcloud credentials/i)).toBeInTheDocument();
      expect(await screen.findByText("nate@example.com")).toBeInTheDocument();
      const discover = screen.getByRole("button", { name: /discover projects/i });
      expect(discover).toBeEnabled();
      await user.clear(screen.getByDisplayValue("gcp"));
      expect(discover).toBeDisabled();
      await user.type(screen.getByRole("textbox"), "work");
      await user.click(discover);
      expect(await heading("One session ready")).toBeInTheDocument();
      expect(add).toHaveBeenCalledWith("work");
      expect(screen.getByText("data-platform")).toBeInTheDocument();
    });

    it("signs in with gcloud when credentials are missing", async () => {
      const user = userEvent.setup();
      vi.spyOn(api, "GCPStatus").mockResolvedValueOnce(gcpNotReady).mockResolvedValue(gcpReady);
      const login = vi.spyOn(api, "GCloudLogin").mockResolvedValue();
      const success = vi.spyOn(toast, "success");
      renderOnboarding();
      expect(await screen.findByText(/no application default credentials yet/i)).toBeInTheDocument();
      expect(screen.getByRole("button", { name: /discover projects/i })).toBeDisabled();
      await user.click(screen.getByRole("button", { name: /sign in with gcloud/i }));
      expect(await screen.findByText("nate@example.com")).toBeInTheDocument();
      expect(login).toHaveBeenCalled();
      expect(success).toHaveBeenCalledWith("Signed in as nate@example.com");
      expect(screen.getByRole("button", { name: /discover projects/i })).toBeEnabled();
    });

    it("rechecks on demand and copies the login command", async () => {
      const user = userEvent.setup();
      vi.spyOn(api, "GCPStatus").mockResolvedValue(gcpNotReady);
      const info = vi.spyOn(toast, "info");
      const success = vi.spyOn(toast, "success");
      renderOnboarding();
      await user.click(await screen.findByRole("button", { name: /check again/i }));
      await waitFor(() => expect(info).toHaveBeenCalledWith("Still no credentials", expect.anything()));
      expect(api.GCPStatus).toHaveBeenCalledTimes(2);
      await user.click(screen.getByRole("button", { name: "Copy command" }));
      await waitFor(() => expect(success).toHaveBeenCalledWith("Command copied", expect.anything()));
      expect(await navigator.clipboard.readText()).toBe("gcloud auth application-default login");
    });

    it("points to the installer when gcloud is missing", async () => {
      const user = userEvent.setup();
      vi.spyOn(api, "GCPStatus").mockResolvedValue({ ...gcpNotReady, gcloudFound: false, gcloudPath: "" });
      const open = vi.spyOn(api, "OpenURL").mockResolvedValue();
      renderOnboarding();
      await user.click(await screen.findByRole("button", { name: /install the gcloud cli/i }));
      expect(open).toHaveBeenCalledWith("https://cloud.google.com/sdk/docs/install");
      expect(screen.queryByRole("button", { name: /sign in with gcloud/i })).not.toBeInTheDocument();
    });

    it("reports a failed gcloud sign-in", async () => {
      const user = userEvent.setup();
      vi.spyOn(api, "GCPStatus").mockResolvedValue(gcpNotReady);
      vi.spyOn(api, "GCloudLogin").mockRejectedValue(new Error("gcloud exited 1"));
      const error = vi.spyOn(toast, "error");
      renderOnboarding();
      await user.click(await screen.findByRole("button", { name: /sign in with gcloud/i }));
      await waitFor(() => expect(error).toHaveBeenCalledWith("gcloud exited 1"));
      expect(screen.getByRole("button", { name: /sign in with gcloud/i })).toBeEnabled();
    });

    it("stays on the step when project discovery fails", async () => {
      const user = userEvent.setup();
      vi.spyOn(api, "GCPStatus").mockResolvedValue(gcpReady);
      vi.spyOn(api, "AddGCP").mockRejectedValue(new Error("no projects"));
      const error = vi.spyOn(toast, "error");
      renderOnboarding();
      await user.click(await screen.findByRole("button", { name: /discover projects/i }));
      await waitFor(() => expect(error).toHaveBeenCalledWith("no projects"));
      expect(screen.getByRole("heading", { level: 2 })).toHaveTextContent("Connect to Google Cloud");
    });
  });

  describe("found on this machine", () => {
    const engineering = {
      alias: "Engineering",
      startUrl: "https://engineering.awsapps.com/start",
      region: "us-east-1",
      profiles: ["AWS_Dev"],
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
    const tenant = { tenantId: "72f988bf", account: "nate@contoso.com", source: "az" };
    const found = {
      awsPortals: [engineering, globex],
      azureTenants: [tenant],
      gcp: { account: "nate@example.com" },
      leapp: { iamUsers: [{ name: "personal-old" }], chainedRoles: [], ssoRoles: 3 },
    };
    const importButton = (title: string) =>
      within(screen.getByText(title).closest("li")!).getByRole("button", { name: /import/i });

    beforeEach(() => {
      at("?step=cloud");
      vi.spyOn(api, "Discover").mockResolvedValue(found as never);
    });

    it("lists what other tools set up and flags each cloud as found", async () => {
      renderOnboarding();
      expect(await screen.findByText("Found on this machine")).toBeInTheDocument();
      expect(screen.getByText("Engineering")).toBeInTheDocument();
      expect(screen.getByText("Signed in via AWS CLI")).toBeInTheDocument();
      expect(screen.getByText("From Granted")).toBeInTheDocument();
      expect(screen.getByText("nate@contoso.com")).toBeInTheDocument();
      expect(screen.getByText("1 Leapp session")).toBeInTheDocument();
      expect(screen.getAllByText("Found")).toHaveLength(3);
      expect(screen.getByText("Or connect something new")).toBeInTheDocument();
    });

    it("reuses an AWS CLI sign-in without a browser round trip", async () => {
      const user = userEvent.setup();
      const sessions = [ssoRole("Engineering Prod/AdministratorAccess"), ssoRole("Engineering Dev/PowerUserAccess")];
      const imp = vi.spyOn(api, "ImportAWSSSO").mockResolvedValue({
        integration: { id: "eng", alias: "Engineering", cloud: "aws" },
        loggedIn: true,
        sessions,
      } as never);
      const success = vi.spyOn(toast, "success");
      renderOnboarding();
      await screen.findByText("Engineering");
      await user.click(importButton("Engineering"));
      expect(await heading("2 sessions ready")).toBeInTheDocument();
      expect(imp).toHaveBeenCalledWith("Engineering", "https://engineering.awsapps.com/start", "us-east-1");
      expect(success).toHaveBeenCalledWith("Reused your AWS CLI sign-in for Engineering");
      expect(screen.getByText("Engineering Prod/AdministratorAccess")).toBeInTheDocument();
    });

    it("runs the device flow for a portal without a token", async () => {
      const user = userEvent.setup();
      const wait = deferred<Session[]>();
      vi.spyOn(api, "ImportAWSSSO").mockResolvedValue({
        integration: { id: "glx", alias: "globex", cloud: "aws" },
        loggedIn: false,
        sessions: [],
      } as never);
      vi.spyOn(api, "StartSSOLogin").mockResolvedValue({ verificationUri: "https://device.example", userCode: "" });
      vi.spyOn(api, "WaitSSOLogin").mockReturnValue(wait.promise as never);
      renderOnboarding();
      await screen.findByText("globex");
      await user.click(importButton("globex"));
      expect(await heading("Approve in your browser")).toBeInTheDocument();
      // No device code: the page asks for approval only.
      expect(screen.getByText(/approve the request there/i)).toBeInTheDocument();
      expect(api.StartSSOLogin).toHaveBeenCalledWith("glx");
      wait.resolve([ssoRole("Globex Prod/Admin")]);
      expect(await heading("One session ready")).toBeInTheDocument();
    });

    it("imports an Azure tenant under its account domain", async () => {
      const user = userEvent.setup();
      const login = deferred<Session[]>();
      const add = vi
        .spyOn(api, "AddAzure")
        .mockResolvedValue({ id: "az1", alias: "contoso", cloud: "azure" } as Integration);
      vi.spyOn(api, "AzureLogin").mockReturnValue(login.promise as never);
      renderOnboarding();
      await screen.findByText("nate@contoso.com");
      await user.click(importButton("nate@contoso.com"));
      expect(await heading("Sign in with Microsoft")).toBeInTheDocument();
      expect(add).toHaveBeenCalledWith("contoso", "72f988bf");
      login.resolve([azureSub]);
      expect(await heading("One session ready")).toBeInTheDocument();
    });

    it("imports the gcloud account", async () => {
      const user = userEvent.setup();
      const add = vi.spyOn(api, "AddGCP").mockResolvedValue([gcpProject]);
      renderOnboarding();
      await screen.findByText("nate@example.com");
      await user.click(importButton("nate@example.com"));
      expect(await heading("One session ready")).toBeInTheDocument();
      expect(add).toHaveBeenCalledWith("gcp");
    });

    it("imports Leapp sessions and reports the skipped ones", async () => {
      const user = userEvent.setup();
      vi.spyOn(api, "ImportLeappSessions").mockResolvedValue({
        sessions: [ssoRole("personal-old", { kind: "aws-iam-user", aws: {} })],
        skipped: ["prod-admin-old: source session missing"],
      } as never);
      const info = vi.spyOn(toast, "info");
      renderOnboarding();
      await screen.findByText("1 Leapp session");
      await user.click(importButton("1 Leapp session"));
      expect(await heading("One session ready")).toBeInTheDocument();
      expect(info).toHaveBeenCalledWith("1 Leapp session skipped", {
        description: "prod-admin-old: source session missing",
      });
    });

    it("stays on the cloud step when an import fails", async () => {
      const user = userEvent.setup();
      vi.spyOn(api, "ImportAWSSSO").mockRejectedValue(new Error("config unreadable"));
      const error = vi.spyOn(toast, "error");
      renderOnboarding();
      await screen.findByText("Engineering");
      await user.click(importButton("Engineering"));
      await waitFor(() => expect(error).toHaveBeenCalledWith("config unreadable"));
      expect(screen.getByRole("heading", { level: 2 })).toHaveTextContent("Where do your roles live?");
      expect(importButton("Engineering")).toBeEnabled();
    });
  });

  describe("roles and finish", () => {
    it("offers another cloud after the first sign-in, then finishes", async () => {
      const user = userEvent.setup();
      at("?step=connect&cloud=azure");
      vi.spyOn(api, "AddAzure").mockResolvedValue({ id: "az1", alias: "contoso", cloud: "azure" } as Integration);
      vi.spyOn(api, "AzureLogin").mockResolvedValue([azureSub]);
      renderOnboarding();
      await user.type(screen.getByPlaceholderText("contoso"), "contoso");
      await user.click(screen.getByRole("button", { name: /sign in with microsoft/i }));
      expect(await heading("One session ready")).toBeInTheDocument();
      await user.click(screen.getByRole("button", { name: /add another cloud/i }));
      expect(await heading("Add another cloud?")).toBeInTheDocument();
      // The cloud just connected shows the chip even before the workspace reloads.
      const azure = screen.getByRole("button", { name: /microsoft azure/i });
      expect(within(azure).getByText("Connected")).toBeInTheDocument();
      await user.click(screen.getByRole("button", { name: /i'm done, finish setup/i }));
      expect(await heading("You're all set.")).toBeInTheDocument();
      expect(screen.getByText("1 session ready to start from the dashboard.")).toBeInTheDocument();
    });

    it("finishes from the roles step", async () => {
      const user = userEvent.setup();
      at("?step=roles");
      renderOnboarding({ ...empty, sessions: [ssoRole("Acme Prod/Admin"), ssoRole("Acme Dev/Power")] });
      expect(screen.getByRole("heading", { level: 2 })).toHaveTextContent("2 sessions ready");
      await user.click(screen.getByRole("button", { name: /^finish/i }));
      expect(await heading("You're all set.")).toBeInTheDocument();
      expect(screen.getByText("2 sessions ready to start from the dashboard.")).toBeInTheDocument();
    });

    it("explains an empty roles step", () => {
      at("?step=roles");
      renderOnboarding();
      expect(screen.getByRole("heading", { level: 2 })).toHaveTextContent("0 sessions ready");
      expect(screen.getByText(/no roles found yet/i)).toBeInTheDocument();
    });

    it("completes onboarding from the final step", async () => {
      const user = userEvent.setup();
      at("?step=done");
      const complete = vi.spyOn(api, "CompleteOnboarding").mockResolvedValue();
      renderOnboarding();
      expect(screen.getByText(/your workspace is ready/i)).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Skip" })).toBeDisabled();
      await user.click(screen.getByRole("button", { name: /open dashboard/i }));
      expect(complete).toHaveBeenCalledTimes(1);
    });

    it("reports a failed completion", async () => {
      const user = userEvent.setup();
      at("?step=done");
      vi.spyOn(api, "CompleteOnboarding").mockRejectedValue(new Error("workspace locked"));
      const error = vi.spyOn(toast, "error");
      renderOnboarding();
      await user.click(screen.getByRole("button", { name: /open dashboard/i }));
      await waitFor(() => expect(error).toHaveBeenCalledWith("workspace locked"));
    });
  });

  describe("navigation", () => {
    it("moves back and forward with the bracket shortcuts", async () => {
      const user = userEvent.setup();
      renderOnboarding();
      await user.keyboard("{Meta>}]{/Meta}");
      expect(await heading("Where do your roles live?")).toBeInTheDocument();
      await user.keyboard("{Control>}]{/Control}");
      expect(await heading("Connect to AWS")).toBeInTheDocument();
      await user.keyboard("{Meta>}[[{/Meta}");
      expect(await heading("Where do your roles live?")).toBeInTheDocument();
      await user.keyboard("{Control>}[[{/Control}");
      expect(await screen.findByRole("heading", { level: 1 })).toHaveTextContent("Assume any role, any cloud.");
      // The welcome step has nothing behind it.
      await user.keyboard("{Meta>}[[{/Meta}");
      expect(screen.getByRole("heading", { level: 1 })).toBeInTheDocument();
    });

    it("moves with Alt and the arrow keys", async () => {
      const user = userEvent.setup();
      at("?step=connect&cloud=aws");
      renderOnboarding();
      await user.keyboard("{Alt>}{ArrowLeft}{/Alt}");
      expect(await heading("Where do your roles live?")).toBeInTheDocument();
      await user.keyboard("{Alt>}{ArrowRight}{/Alt}");
      expect(await heading("Connect to AWS")).toBeInTheDocument();
    });

    it("moves with the mouse back and forward buttons", async () => {
      at("?step=connect&cloud=aws");
      renderOnboarding();
      fireEvent.mouseUp(window, { button: 3 });
      expect(await heading("Where do your roles live?")).toBeInTheDocument();
      fireEvent.mouseUp(window, { button: 4 });
      expect(await heading("Connect to AWS")).toBeInTheDocument();
    });

    it("follows browser history", async () => {
      const user = userEvent.setup();
      renderOnboarding();
      await user.click(screen.getByRole("button", { name: /get started/i }));
      expect(await heading("Where do your roles live?")).toBeInTheDocument();
      act(() => window.history.back());
      expect(await screen.findByRole("heading", { level: 1 })).toHaveTextContent("Assume any role, any cloud.");
      act(() => window.history.forward());
      expect(await heading("Where do your roles live?")).toBeInTheDocument();
    });

    it("never skips forward into a step that needs a sign-in", async () => {
      const user = userEvent.setup();
      at("?step=connect&cloud=aws");
      renderOnboarding();
      await user.keyboard("{Meta>}]{/Meta}");
      expect(screen.getByRole("heading", { level: 2 })).toHaveTextContent("Connect to AWS");
    });

    it("goes back from the roles step to the cloud step", async () => {
      const user = userEvent.setup();
      at("?step=roles");
      renderOnboarding({ ...empty, sessions: [azureSub] });
      await user.keyboard("{Meta>}[[{/Meta}");
      expect(await heading("Add another cloud?")).toBeInTheDocument();
      await user.keyboard("{Meta>}]{/Meta}");
      expect(await heading("Connect to AWS")).toBeInTheDocument();
    });

    it("abandons a pending approval when the user goes back", async () => {
      const user = userEvent.setup();
      at("?step=connect&cloud=aws");
      const { cancel } = stubAWSLogin();
      renderOnboarding();
      await submitSSO(user);
      expect(await heading("Approve in your browser")).toBeInTheDocument();
      // Forward has no target while a sign-in waits.
      await user.keyboard("{Meta>}]{/Meta}");
      expect(screen.getByRole("heading", { level: 2 })).toHaveTextContent("Approve in your browser");
      await user.keyboard("{Meta>}[[{/Meta}");
      expect(await heading("Connect to AWS")).toBeInTheDocument();
      expect(cancel).toHaveBeenCalledWith("i1");
    });

    it("ignores navigation while a request is in flight", async () => {
      const user = userEvent.setup();
      at("?step=connect&cloud=aws");
      const add = deferred<Integration>();
      vi.spyOn(api, "AddAWSSSO").mockReturnValue(add.promise as never);
      vi.spyOn(toast, "error");
      renderOnboarding();
      await submitSSO(user);
      await user.keyboard("{Meta>}[[{/Meta}");
      expect(screen.getByRole("heading", { level: 2 })).toHaveTextContent("Connect to AWS");
      add.reject(new Error("slow"));
      await waitFor(() => expect(screen.getByRole("button", { name: /^sign in$/i })).toBeEnabled());
    });
  });
});
