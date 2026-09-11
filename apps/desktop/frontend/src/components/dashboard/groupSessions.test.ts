import { describe, expect, it } from "vitest";
import { Kind } from "@/lib/api";
import { groupSessions } from "@/components/dashboard/SessionTable";
import { session, ssoRole } from "@/test/fixtures";

describe("groupSessions", () => {
  it("groups SSO roles by integration and account, labelled by the name prefix", () => {
    const rows = groupSessions([
      ssoRole("Acme Prod", "111", "ReadOnly"),
      ssoRole("Acme Prod", "111", "Admin"),
      ssoRole("Acme Dev", "222", "PowerUser"),
    ]);
    expect(rows).toHaveLength(2);
    expect(rows.map((r) => r.label)).toEqual(["Acme Dev", "Acme Prod"]);
    const prod = rows[1];
    expect(prod.group).toBe(true);
    if (!prod.group) throw new Error("expected a group");
    expect(prod.accountId).toBe("111");
    expect(prod.key).toBe("acme:111");
    expect(prod.sessions.map((s) => s.aws?.roleName)).toEqual(["Admin", "ReadOnly"]);
  });

  it("keeps the same account apart when it comes from different integrations", () => {
    const rows = groupSessions([
      ssoRole("Acme Prod", "111", "Admin"),
      ssoRole("Acme Prod", "111", "Admin", { integrationId: "acme-eu" }),
    ]);
    expect(rows).toHaveLength(2);
    expect(rows.map((r) => r.key).sort()).toEqual(["acme-eu:111", "acme:111"]);
  });

  it("keeps the account label when the first role was renamed", () => {
    const [row] = groupSessions([
      ssoRole("Acme Prod", "111", "Admin", { name: "prod-admin" }),
      ssoRole("Acme Prod", "111", "ReadOnly"),
    ]);
    expect(row.label).toBe("Acme Prod");
  });

  it("falls back to the account id when the name has no prefix", () => {
    const [row] = groupSessions([ssoRole("", "333", "Admin")]);
    expect(row.label).toBe("333");
  });

  it("gives standalone rows to IAM users, assume roles, Azure and GCP", () => {
    const rows = groupSessions([
      session({ name: "personal", kind: Kind.KindAWSIAMUser }),
      session({ name: "prod-admin", kind: Kind.KindAWSAssumeRole, aws: { roleArn: "arn" } }),
      session({ name: "Contoso", kind: Kind.KindAzure }),
      session({ name: "data", kind: Kind.KindGCP }),
    ]);
    expect(rows.every((r) => !r.group)).toBe(true);
    expect(rows.map((r) => r.label)).toEqual(["personal", "prod-admin", "Contoso", "data"]);
  });

  it("does not group an SSO role that lacks an account id", () => {
    const [row] = groupSessions([session({ name: "orphan", kind: Kind.KindAWSSSORole, aws: {} })]);
    expect(row.group).toBe(false);
  });

  it("orders aws, then azure, then gcp, alphabetically within each cloud", () => {
    const rows = groupSessions([
      session({ name: "zeta-project", kind: Kind.KindGCP }),
      session({ name: "Beta Tenant", kind: Kind.KindAzure }),
      session({ name: "zulu-user", kind: Kind.KindAWSIAMUser }),
      ssoRole("Alpha Corp", "111", "Admin"),
      session({ name: "alpha-project", kind: Kind.KindGCP }),
      session({ name: "Alpha Tenant", kind: Kind.KindAzure }),
    ]);
    expect(rows.map((r) => r.label)).toEqual([
      "Alpha Corp",
      "zulu-user",
      "Alpha Tenant",
      "Beta Tenant",
      "alpha-project",
      "zeta-project",
    ]);
  });

  it("sorts roles inside a group by role name and falls back to the session name", () => {
    const rows = groupSessions([
      ssoRole("Acme", "111", "ReadOnly"),
      ssoRole("Acme", "111", "Billing"),
      ssoRole("Acme", "111", "Admin"),
    ]);
    const g = rows[0];
    if (!g.group) throw new Error("expected a group");
    expect(g.sessions.map((s) => s.name)).toEqual(["Acme/Admin", "Acme/Billing", "Acme/ReadOnly"]);
  });
});
