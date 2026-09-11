// Browser-only mock of the Go service so the UI can be developed with
// `aube run dev` outside Wails. Never bundled into the desktop app path.
import { Status, type Credentials, type Integration, type Session, type Workspace as CoreWorkspace } from "../../bindings/github.com/nateships/rolle/internal/core";
type Workspace = Omit<CoreWorkspace, "sessions" | "integrations"> & { sessions: Session[]; integrations: Integration[] };

const wait = (ms: number) => new Promise((r) => setTimeout(r, ms));
const id = () => Math.random().toString(36).slice(2, 10);
const listeners = new Set<() => void>();
const emit = () => listeners.forEach((l) => l());

const state: Workspace = {
  version: 1,
  onboarded: new URLSearchParams(location.search).get("view") === "dashboard",
  integrations: [],
  sessions: [],
  settings: { theme: (new URLSearchParams(location.search).get("theme") ?? "system"), defaultRegion: "us-east-1", assumeRoleMinutes: 60, hideOnClose: true, verboseLogging: false },
} as unknown as Workspace;

function seed() {
  const sso: Integration = { id: "acme", alias: "acme", cloud: "aws", awsSso: { startUrl: "https://acme.awsapps.com/start", region: "us-east-1", tokenExpires: new Date(Date.now() + 6e6).toISOString() } } as Integration;
  const az: Integration = { id: "contoso", alias: "contoso", cloud: "azure", azure: { tenantId: "t-1", account: "nate@contoso.com" } } as Integration;
  const gcp: Integration = { id: "gcp", alias: "gcp", cloud: "gcp", gcp: { account: "nate@example.com" } } as Integration;
  state.integrations = [sso, az, gcp];
  const s = (o: Record<string, unknown>): Session => ({ id: id(), status: "inactive", ...o }) as unknown as Session;
  state.sessions = [
    s({ name: "Acme Prod/AdministratorAccess", kind: "aws-sso-role", region: "us-east-1", integrationId: "acme", aws: { accountId: "123456789012", roleName: "AdministratorAccess" }, status: "active", favorite: true, expires: new Date(Date.now() + 47 * 60e3).toISOString() }),
    s({ name: "Acme Dev/PowerUser", kind: "aws-sso-role", region: "us-east-1", integrationId: "acme", aws: { accountId: "210987654321", roleName: "PowerUserAccess" } }),
    s({ name: "prod-admin", kind: "aws-assume-role", region: "eu-west-1", aws: { roleArn: "arn:aws:iam::123456789012:role/Admin", sourceSessionId: "x" } }),
    s({ name: "personal", kind: "aws-iam-user", region: "us-west-2", aws: { mfaDevice: "arn:aws:iam::1:mfa/me" } }),
    s({ name: "Contoso Production", kind: "azure", integrationId: "contoso", azure: { subscriptionId: "0f1e2d3c-4b5a-6978-8a9b-0c1d2e3f4a5b", tenantId: "t-1" }, status: "active", expires: new Date(Date.now() + 3.2e6).toISOString() }),
    s({ name: "data-platform", kind: "gcp", integrationId: "gcp", gcp: { projectId: "data-platform-4821" } }),
    s({ name: "deployer", kind: "gcp", integrationId: "gcp", favorite: true, gcp: { projectId: "data-platform-4821", serviceAccount: "deployer@data-platform-4821.iam.gserviceaccount.com" } }),
  ];
}
if (state.onboarded) seed();

// ?gcp=missing simulates a machine without gcloud credentials.
let gcpReady = new URLSearchParams(location.search).get("gcp") !== "missing";
// ?gcloud=none simulates a machine without the gcloud CLI.
const gcloudFound = new URLSearchParams(location.search).get("gcloud") !== "none";

const creds: Credentials = { accessKeyId: "ASIAMOCK", secretAccessKey: "mock", sessionToken: "mock", expiration: new Date(Date.now() + 3.6e6).toISOString() } as Credentials;

export const mockApi = {
  Workspace: async () => structuredClone(state),
  CompleteOnboarding: async () => { state.onboarded = true; if (state.sessions.length === 0) seed(); emit(); },
  AddAWSSSO: async (alias: string, startUrl: string, region: string) => { const i = { id: id(), alias, cloud: "aws", awsSso: { startUrl, region } } as Integration; state.integrations.push(i); emit(); return i; },
  StartSSOLogin: async () => { await wait(600); return { verificationUri: "https://device.sso.us-east-1.amazonaws.com/?user_code=MOCK-CODE", userCode: "MOCK-CODE" }; },
  WaitSSOLogin: async (ref: string) => { await wait(2500); const i = state.integrations.find((x) => x.id === ref)!; i.awsSso!.tokenExpires = new Date(Date.now() + 8 * 3.6e6).toISOString(); const added = ["Acme Prod/AdministratorAccess", "Acme Dev/PowerUser", "Acme Sandbox/ReadOnly"].map((name) => ({ id: id(), name, kind: "aws-sso-role", region: "us-east-1", integrationId: ref, status: "inactive", aws: { accountId: "123456789012", roleName: name.split("/")[1] } }) as Session); state.sessions.push(...added); emit(); return added; },
  SSOLogout: async (ref: string) => { const i = state.integrations.find((x) => x.id === ref); if (i?.awsSso) i.awsSso.tokenExpires = null; emit(); },
  SyncSSO: async () => { await wait(500); return []; },
  RemoveIntegration: async (ref: string) => { state.integrations = state.integrations.filter((i) => i.id !== ref); state.sessions = state.sessions.filter((s) => s.integrationId !== ref); emit(); },
  AddAssumeRole: async (v: { name: string; roleArn: string; region: string; sourceRef: string }) => { const s = { id: id(), name: v.name, kind: "aws-assume-role", region: v.region, status: "inactive", aws: { roleArn: v.roleArn, sourceSessionId: v.sourceRef } } as Session; state.sessions.push(s); emit(); return s; },
  AddIAMUser: async (v: { name: string; region: string; mfaDevice: string }) => { const s = { id: id(), name: v.name, kind: "aws-iam-user", region: v.region, status: "inactive", aws: { mfaDevice: v.mfaDevice } } as Session; state.sessions.push(s); emit(); return s; },
  RemoveSession: async (ref: string) => { state.sessions = state.sessions.filter((s) => s.id !== ref); emit(); },
  Start: async (ref: string) => { await wait(700); const s = state.sessions.find((x) => x.id === ref)!; s.status = Status.StatusActive; s.expires = new Date(Date.now() + 3.6e6).toISOString(); emit(); return creds; },
  Stop: async (ref: string) => { const s = state.sessions.find((x) => x.id === ref)!; s.status = Status.StatusInactive; s.expires = null; emit(); },
  Credentials: async () => creds,
  ProfileName: async (ref: string) => (state.sessions.find((x) => x.id === ref)?.name ?? "").replace(/[^A-Za-z0-9._-]/g, "-"),
  EnvText: async () => "export AWS_ACCESS_KEY_ID=ASIAMOCK\n",
  OpenConsole: async () => {},
  OpenURL: async (u: string) => { window.open(u, "_blank"); },
  AddAzure: async (alias: string, tenantId: string) => { const i = { id: id(), alias, cloud: "azure", azure: { tenantId } } as Integration; state.integrations.push(i); emit(); return i; },
  AzureLogin: async (ref: string) => { await wait(2500); const i = state.integrations.find((x) => x.id === ref)!; i.azure!.account = "nate@contoso.com"; const added = [{ id: id(), name: "Contoso Production", kind: "azure", integrationId: ref, status: "inactive", azure: { subscriptionId: "0f1e2d3c-4b5a-6978-8a9b-0c1d2e3f4a5b", tenantId: "t-1" } }] as Session[]; state.sessions.push(...added); emit(); return added; },
  AzureLogout: async (ref: string) => { const i = state.integrations.find((x) => x.id === ref); if (i?.azure) i.azure.account = ""; emit(); },
  SyncAzure: async () => [],
  GCPStatus: async () => { await wait(500); return { ready: gcpReady, account: gcpReady ? "nate@example.com" : "", loginCommand: "gcloud auth application-default login", gcloudFound, gcloudPath: gcloudFound ? "/opt/homebrew/bin/gcloud" : "", installUrl: "https://cloud.google.com/sdk/docs/install" }; },
  GCloudLogin: async () => { await wait(1500); gcpReady = true; },
  AddGCP: async (alias: string) => { await wait(800); const i = { id: id(), alias, cloud: "gcp", gcp: { account: "nate@example.com" } } as Integration; state.integrations.push(i); const added = [{ id: id(), name: "data-platform", kind: "gcp", integrationId: i.id, status: "inactive", gcp: { projectId: "data-platform-4821" } }] as Session[]; state.sessions.push(...added); emit(); return added; },
  SyncGCP: async () => [],
  AddGCPImpersonation: async (v: { name: string; integrationRef: string; projectId: string; serviceAccount: string }) => { const s = { id: id(), name: v.name, kind: "gcp", integrationId: v.integrationRef, status: "inactive", gcp: { projectId: v.projectId, serviceAccount: v.serviceAccount } } as Session; state.sessions.push(s); emit(); return s; },
  Settings: async () => ({ ...state.settings! }),
  UpdateSettings: async (v: NonNullable<Workspace["settings"]>) => { state.settings = { ...v }; emit(); return { ...v }; },
  Info: async () => ({ version: "0.0.1-dev", workspacePath: "~/.config/rolle/workspace.json", cacheDir: "~/.cache/rolle/credentials", awsConfigPath: "~/.aws/config" }),
  ReplayOnboarding: async () => { state.onboarded = false; emit(); },
  Reset: async () => { state.onboarded = false; state.integrations = []; state.sessions = []; emit(); },
  Discover: async () => { await wait(400); const q = new URLSearchParams(location.search); if (q.get("found") === "none") return { awsPortals: [], azureTenants: [], gcp: null }; return {
    awsPortals: [
      { alias: "Engineering", startUrl: "https://engineering.awsapps.com/start", region: "us-east-1", profiles: ["AWS_Dev", "AWS_Prod"], hasToken: true },
      { alias: "timescale", startUrl: "https://timescale.awsapps.com/start", region: "us-west-2", profiles: ["timescale-prod"], hasToken: false },
    ],
    azureTenants: [{ tenantId: "72f988bf-86f1-41af-91ab-2d7cd011db47", account: "nate@contoso.com" }],
    gcp: { account: "nate@example.com" },
  }; },
  ImportAWSSSO: async (alias: string, startUrl: string, region: string) => { await wait(900); const i = { id: id(), alias, cloud: "aws", awsSso: { startUrl, region, tokenExpires: new Date(Date.now() + 8 * 3.6e6).toISOString() } } as Integration; state.integrations.push(i); const loggedIn = startUrl.includes("engineering"); const sessions = loggedIn ? ["Engineering Prod/AdministratorAccess", "Engineering Dev/PowerUserAccess"].map((name) => ({ id: id(), name, kind: "aws-sso-role", region, integrationId: i.id, status: "inactive", aws: { accountId: "123456789012", roleName: name.split("/")[1] } }) as unknown as Session) : []; state.sessions.push(...sessions); emit(); return { integration: i, loggedIn, sessions }; },
  SetFavorite: async (ref: string, fav: boolean) => { const x = state.sessions.find((s) => s.id === ref); if (x) (x as unknown as { favorite: boolean }).favorite = fav; emit(); },
  RenameSession: async (ref: string, name: string) => { const x = state.sessions.find((s) => s.id === ref); if (x) x.name = name; emit(); },
  RenameIntegration: async (ref: string, alias: string) => { const x = state.integrations.find((i) => i.id === ref); if (x) x.alias = alias; emit(); },
  DevMode: async () => true,
  onChange: (cb: () => void) => { listeners.add(cb); return () => listeners.delete(cb); },
};
