import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import {
  FoundList,
  tenantAlias,
  type FoundIAMUser,
  type FoundPortal,
  type FoundTenant,
} from "@/components/discovery/FoundList";
import { api } from "@/lib/api";

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
    const remove = vi.spyOn(api, "RemoveStaticProfile").mockResolvedValue();
    render(<FoundList {...base} iamUsers={[iamUser()]} />);
    expect(screen.queryByRole("button", { name: /import all/i })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /IAM users in the credentials file/ }));
    await user.click(rowOf("personal").getByRole("button", { name: /import/i }));
    await waitFor(() => expect(imp).toHaveBeenCalledWith("personal"));
    const dialog = await screen.findByRole("dialog", { name: "Remove profile from the credentials file?" });
    expect(dialog).toHaveTextContent("[personal]");
    expect(remove).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole("button", { name: "Remove" }));
    await waitFor(() => expect(remove).toHaveBeenCalledWith("personal"));
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
    render(<FoundList {...base} iamUsers={[iamUser(), iamUser({ profile: "plain" })]} />);
    // One user shows no Import all; two do.
    await user.click(screen.getByRole("button", { name: /import all/i }));
    await waitFor(() => expect(imp).toHaveBeenCalledTimes(2));
    expect(imp).toHaveBeenCalledWith("personal");
    expect(imp).toHaveBeenCalledWith("plain");
    const dialog = await screen.findByRole("dialog", { name: "Remove profiles from the credentials file?" });
    expect(dialog).toHaveTextContent("[personal]");
    expect(dialog).toHaveTextContent("[plain]");
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
