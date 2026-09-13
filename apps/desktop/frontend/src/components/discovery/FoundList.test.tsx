import { act, fireEvent, render, renderHook, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { describe, expect, it, vi } from "vitest";
import {
  FoundList,
  tenantAlias,
  useDiscovery,
  type FoundIAMUser,
  type FoundPortal,
  type FoundTenant,
} from "@/components/discovery/FoundList";
import { api, type Workspace } from "@/lib/api";

const portal = (o: Partial<FoundPortal> = {}): FoundPortal => ({
  alias: "Engineering",
  startUrl: "https://engineering.awsapps.com/start",
  region: "us-east-1",
  profiles: [],
  hasToken: false,
  source: "aws-cli",
  ...o,
});
const tenant = (o: Partial<FoundTenant> = {}): FoundTenant => ({
  tenantId: "t-1",
  account: "nate@contoso.com",
  source: "az",
  ...o,
});

const iamUser = (o: Partial<FoundIAMUser> = {}): FoundIAMUser => ({
  profile: "personal",
  accessKeyId: "AKIAIOSFODNN7EXAMPLE",
  region: "us-west-2",
  mfaDevice: "arn:aws:iam::1:mfa/me",
  imported: false,
  ...o,
});

const noop = { onAWS: vi.fn(), onAzure: vi.fn(), onGCP: vi.fn(), onLeapp: vi.fn() };
const base = { portals: [], tenants: [], gcp: null, leapp: null, importing: null, disabled: false, ...noop };

const rowOf = (title: string) => within(screen.getByText(title).closest("li")!);

describe("FoundList", () => {
  it("describes each AWS portal by sign-in state, source, and profile count", () => {
    render(
      <FoundList
        {...base}
        portals={[
          portal({ hasToken: true, profiles: ["a", "b"] }),
          portal({ alias: "globex", startUrl: "https://globex/start", source: "granted", profiles: ["p"] }),
          portal({ alias: "other", startUrl: "https://other/start", source: "custom-tool" }),
        ]}
      />,
    );
    expect(rowOf("Engineering").getByText("Signed in via AWS CLI")).toBeInTheDocument();
    expect(rowOf("Engineering").getByText(/2 profiles/)).toBeInTheDocument();
    expect(rowOf("globex").getByText("From Granted")).toBeInTheDocument();
    expect(rowOf("globex").getByText(/1 profile$/)).toBeInTheDocument();
    // An unknown source shows as-is.
    expect(rowOf("other").getByText("From custom-tool")).toBeInTheDocument();
    expect(rowOf("other").queryByText(/profile/)).not.toBeInTheDocument();
  });

  it("falls back to generic names for a tenant and a Google account without one", () => {
    render(<FoundList {...base} tenants={[tenant({ account: "" })]} gcp={{ account: "" }} />);
    expect(rowOf("Azure tenant").getByText("t-1")).toBeInTheDocument();
    expect(rowOf("Azure tenant").getByText("From az CLI")).toBeInTheDocument();
    expect(rowOf("Google account").getByText("Signed in via gcloud")).toBeInTheDocument();
  });

  it("summarises the Leapp sessions", () => {
    render(
      <FoundList
        {...base}
        leapp={{ iamUsers: [{ name: "a" }], chainedRoles: [{ name: "b" }, { name: "c" }], ssoRoles: 5 }}
      />,
    );
    const row = rowOf("3 Leapp sessions");
    expect(
      row.getByText("1 IAM user · 2 chained roles · 5 SSO roles return when you sync the portal"),
    ).toBeInTheDocument();
    expect(row.getByText("From Leapp")).toBeInTheDocument();
  });

  it("hides the Leapp row when there is nothing to import or no handler", () => {
    const view = render(<FoundList {...base} leapp={{ iamUsers: [], chainedRoles: [], ssoRoles: 5 }} />);
    expect(screen.queryByText(/leapp/i)).not.toBeInTheDocument();
    view.unmount();
    render(
      <FoundList {...base} leapp={{ iamUsers: [{ name: "a" }], chainedRoles: [], ssoRoles: 0 }} onLeapp={undefined} />,
    );
    expect(screen.queryByText(/leapp/i)).not.toBeInTheDocument();
  });

  it("lists IAM users by profile, region, and MFA", () => {
    render(<FoundList {...base} iamUsers={[iamUser(), iamUser({ profile: "plain", region: "", mfaDevice: "" })]} />);
    // The group starts folded; the heading opens it.
    expect(screen.queryByText("personal")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /IAM users in the credentials file/ }));
    expect(rowOf("personal").getByText("AKIA… · us-west-2")).toBeInTheDocument();
    expect(rowOf("personal").getByText("MFA")).toBeInTheDocument();
    expect(rowOf("plain").getByText("AKIA…")).toBeInTheDocument();
    expect(rowOf("plain").queryByText("MFA")).not.toBeInTheDocument();
  });

  it("imports an IAM user, then offers to remove the key from the file", async () => {
    const user = userEvent.setup();
    const imp = vi.spyOn(api, "ImportIAMUser").mockResolvedValue({ id: "s1", name: "personal" } as never);
    vi.spyOn(api, "StaticProfiles").mockResolvedValue({
      path: "~/.aws/credentials",
      profiles: [{ name: "personal", keys: [{ name: "aws_access_key_id", preview: "AKIA…" }], imported: true }],
    });
    const imported = vi.fn();
    render(<FoundList {...base} iamUsers={[iamUser()]} onImportedUsers={imported} />);
    expect(screen.queryByRole("button", { name: /import all/i })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /IAM users in the credentials file/ }));
    await user.click(rowOf("personal").getByRole("button", { name: /import/i }));
    await waitFor(() => expect(imp).toHaveBeenCalledWith("personal"));
    // The parent owns the removal offer, since the list rescans and drops the row.
    await waitFor(() => expect(imported).toHaveBeenCalledWith(["personal"]));
  });

  it("imports every IAM user at once, then offers to remove them together", async () => {
    const user = userEvent.setup();
    const imp = vi.spyOn(api, "ImportIAMUser").mockResolvedValue({ id: "s1", name: "x" } as never);
    vi.spyOn(api, "StaticProfiles").mockResolvedValue({
      path: "~/.aws/credentials",
      profiles: [
        { name: "personal", keys: [{ name: "aws_access_key_id", preview: "AKIA…" }], imported: true },
        { name: "plain", keys: [{ name: "aws_access_key_id", preview: "AKIA…" }], imported: true },
      ],
    });
    const imported = vi.fn();
    render(<FoundList {...base} iamUsers={[iamUser(), iamUser({ profile: "plain" })]} onImportedUsers={imported} />);
    // One user shows no Import all; two do.
    await user.click(screen.getByRole("button", { name: /import all/i }));
    await waitFor(() => expect(imp).toHaveBeenCalledTimes(2));
    expect(imp).toHaveBeenCalledWith("personal");
    expect(imp).toHaveBeenCalledWith("plain");
    await waitFor(() => expect(imported).toHaveBeenCalledWith(["personal", "plain"]));
  });

  it("renders an empty list when nothing was found", () => {
    render(<FoundList {...base} />);
    expect(screen.getByRole("list")).toBeEmptyDOMElement();
  });

  it("calls the matching import handler for each row", async () => {
    const user = userEvent.setup();
    const onAWS = vi.fn();
    const onAzure = vi.fn();
    const onGCP = vi.fn();
    const onLeapp = vi.fn();
    const p = portal();
    const t = tenant();
    render(
      <FoundList
        {...base}
        portals={[p]}
        tenants={[t]}
        gcp={{ account: "nate@example.com" }}
        leapp={{ iamUsers: [{ name: "a" }], chainedRoles: [], ssoRoles: 0 }}
        onAWS={onAWS}
        onAzure={onAzure}
        onGCP={onGCP}
        onLeapp={onLeapp}
      />,
    );
    await user.click(rowOf("Engineering").getByRole("button", { name: /import/i }));
    expect(onAWS).toHaveBeenCalledWith(p);
    await user.click(rowOf("nate@contoso.com").getByRole("button", { name: /import/i }));
    expect(onAzure).toHaveBeenCalledWith(t);
    await user.click(rowOf("nate@example.com").getByRole("button", { name: /import/i }));
    expect(onGCP).toHaveBeenCalledTimes(1);
    await user.click(rowOf("1 Leapp session").getByRole("button", { name: /import/i }));
    expect(onLeapp).toHaveBeenCalledTimes(1);
  });

  it("disables every Import button while one import runs", () => {
    render(<FoundList {...base} portals={[portal()]} gcp={{ account: "g" }} importing="gcp" disabled />);
    for (const b of screen.getAllByRole("button", { name: /import/i })) expect(b).toBeDisabled();
  });
});

describe("tenantAlias", () => {
  it("derives the alias from the account", () => {
    expect(tenantAlias(tenant({ account: "nate@contoso.com" }))).toBe("contoso");
    expect(tenantAlias(tenant({ account: "Contoso Ltd" }))).toBe("Contoso Ltd");
    expect(tenantAlias(tenant({ account: "" }))).toBe("azure");
  });
});

describe("FoundList IAM users", () => {
  it("folds the users behind a count and unfolds on the heading", async () => {
    const user = userEvent.setup();
    render(<FoundList {...base} iamUsers={[iamUser(), iamUser({ profile: "plain" })]} />);
    const heading = screen.getByRole("button", { name: /IAM users in the credentials file/ });
    expect(heading).toHaveAttribute("aria-expanded", "false");
    expect(within(heading).getByText("2")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Import all 2" })).toBeInTheDocument();
    expect(screen.queryByText("personal")).not.toBeInTheDocument();
    await user.click(heading);
    expect(heading).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByText("personal")).toBeInTheDocument();
    expect(screen.getByText("plain")).toBeInTheDocument();
    await user.click(heading);
    expect(screen.queryByText("personal")).not.toBeInTheDocument();
  });

  it("stops at the first failed import and still offers to remove the keys that moved", async () => {
    const user = userEvent.setup();
    const imp = vi
      .spyOn(api, "ImportIAMUser")
      .mockResolvedValueOnce({ id: "s1", name: "personal" } as never)
      .mockRejectedValueOnce(new Error("session plain already exists"));
    const error = vi.spyOn(toast, "error");
    const success = vi.spyOn(toast, "success");
    const imported = vi.fn();
    render(<FoundList {...base} iamUsers={[iamUser(), iamUser({ profile: "plain" })]} onImportedUsers={imported} />);
    await user.click(screen.getByRole("button", { name: "Import all 2" }));
    await waitFor(() => expect(error).toHaveBeenCalledWith("session plain already exists"));
    expect(imp).toHaveBeenCalledTimes(2);
    expect(success).not.toHaveBeenCalled();
    // The key that moved is still offered for removal.
    await waitFor(() => expect(imported).toHaveBeenCalledWith(["personal"]));
    expect(screen.getByRole("button", { name: "Import all 2" })).toBeEnabled();
  });

  it("does not offer the removal when no key moved", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "ImportIAMUser").mockRejectedValue(new Error("no access key"));
    const error = vi.spyOn(toast, "error");
    const imported = vi.fn();
    render(<FoundList {...base} iamUsers={[iamUser()]} onImportedUsers={imported} />);
    await user.click(screen.getByRole("button", { name: "Import" }));
    await waitFor(() => expect(error).toHaveBeenCalledWith("no access key"));
    expect(imported).not.toHaveBeenCalled();
  });
});

describe("useDiscovery", () => {
  const workspace = (o: Partial<Workspace> = {}): Workspace =>
    ({ sessions: [], integrations: [], ...o }) as unknown as Workspace;
  const scan = {
    awsPortals: [portal(), portal({ alias: "known", startUrl: "https://known.awsapps.com/start/" })],
    azureTenants: [tenant(), tenant({ tenantId: "t-known" })],
    iamUsers: [iamUser(), iamUser({ profile: "taken" }), iamUser({ profile: "old", imported: true })],
    gcp: { account: "nate@example.com" },
    leapp: { iamUsers: [{ name: "leapp-user" }], chainedRoles: [], ssoRoles: 2 },
  };

  it("drops what the workspace already has", async () => {
    vi.spyOn(api, "Discover").mockResolvedValue(scan as never);
    const w = workspace({
      integrations: [
        { id: "i1", alias: "known", cloud: "aws", awsSso: { startUrl: "https://known.awsapps.com/start" } },
        { id: "i2", alias: "contoso", cloud: "azure", azure: { tenantId: "t-known" } },
      ] as never,
      sessions: [{ id: "s1", name: "taken" }] as never,
    });
    const { result } = renderHook(() => useDiscovery(w));
    expect(result.current.loading).toBe(true);
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.portals.map((p) => p.alias)).toEqual(["Engineering"]);
    expect(result.current.tenants.map((t) => t.tenantId)).toEqual(["t-1"]);
    // Imported keys and profiles a session took by name are not offered.
    expect(result.current.iamUsers.map((u) => u.profile)).toEqual(["personal"]);
    expect(result.current.gcp).toEqual({ account: "nate@example.com" });
    expect(result.current.leapp).toEqual(scan.leapp);
    expect(result.current.count).toBe(5);
  });

  it("hides Google Cloud and Leapp when nothing new is left", async () => {
    vi.spyOn(api, "Discover").mockResolvedValue(scan as never);
    const w = workspace({
      integrations: [{ id: "g", alias: "gcp", cloud: "gcp", gcp: { account: "x" } }] as never,
      sessions: [{ id: "s1", name: "leapp-user" }] as never,
    });
    const { result } = renderHook(() => useDiscovery(w));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.gcp).toBeNull();
    expect(result.current.leapp).toBeNull();
  });

  it("treats Go nil slices as empty and a failed scan as a failure", async () => {
    // A scan can come back with every list null.
    const discover = vi.spyOn(api, "Discover").mockResolvedValue({ leapp: { ssoRoles: 1 } } as never);
    // One workspace object for every render: the hook scans again when it changes.
    const w = workspace();
    const { result } = renderHook(() => useDiscovery(w));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.count).toBe(0);
    expect(result.current.leapp).toBeNull();

    // Scan again reads the machine once more.
    discover.mockRejectedValue(new Error("credentials file unreadable"));
    const error = vi.spyOn(toast, "error");
    act(() => result.current.rescan());
    expect(result.current.loading).toBe(true);
    await waitFor(() =>
      expect(error).toHaveBeenCalledWith("Scan failed", { description: "credentials file unreadable" }),
    );
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.count).toBe(0);
    expect(discover).toHaveBeenCalledTimes(2);
  });

  it("does not scan while disabled", () => {
    const discover = vi.spyOn(api, "Discover").mockResolvedValue(scan as never);
    const w = workspace();
    const { result } = renderHook(() => useDiscovery(w, false));
    expect(discover).not.toHaveBeenCalled();
    expect(result.current.loading).toBe(true);
    expect(result.current.count).toBe(0);
  });
});
