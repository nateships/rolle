import { describe, expect, it } from "vitest";
import { Cloud, Kind } from "@/lib/api";
import { ago, cloudOf, isLoggedIn, kindLabel, remaining, sessionSubtitle } from "@/lib/format";
import { integration, session } from "@/test/fixtures";

const NOW = Date.parse("2026-01-01T12:00:00Z");
const at = (offsetMs: number) => new Date(NOW + offsetMs).toISOString();

describe("remaining", () => {
  it("returns an empty string without an expiry", () => {
    expect(remaining(null, NOW)).toBe("");
    expect(remaining(undefined, NOW)).toBe("");
    expect(remaining("", NOW)).toBe("");
  });

  it("reports expired when the time has passed", () => {
    expect(remaining(at(0), NOW)).toBe("expired");
    expect(remaining(at(-1000), NOW)).toBe("expired");
  });

  it("formats hours and minutes above one hour", () => {
    expect(remaining(at(65 * 60e3 + 30e3), NOW)).toBe("1h 5m");
    expect(remaining(at(2 * 3600e3), NOW)).toBe("2h 0m");
  });

  it("formats minutes and zero-padded seconds below one hour", () => {
    expect(remaining(at(5 * 60e3 + 7e3), NOW)).toBe("5m 07s");
    expect(remaining(at(59 * 60e3 + 59e3), NOW)).toBe("59m 59s");
  });

  it("formats seconds only below one minute", () => {
    expect(remaining(at(42e3), NOW)).toBe("42s");
    expect(remaining(at(999), NOW)).toBe("0s");
  });
});

describe("ago", () => {
  const now = Date.parse("2026-09-15T00:00:00Z");
  it("returns an empty string without a time", () => {
    expect(ago(null, now)).toBe("");
  });
  it("says just now under a minute, including the future", () => {
    expect(ago(new Date(now - 30e3).toISOString(), now)).toBe("just now");
    expect(ago(new Date(now + 30e3).toISOString(), now)).toBe("just now");
  });
  it("formats minutes, then hours and minutes", () => {
    expect(ago(new Date(now - 3 * 60e3).toISOString(), now)).toBe("3m ago");
    expect(ago(new Date(now - (2 * 3600e3 + 5 * 60e3)).toISOString(), now)).toBe("2h 5m ago");
  });
});

describe("cloudOf", () => {
  it("maps each kind to its cloud", () => {
    expect(cloudOf(Kind.KindAzure)).toBe("azure");
    expect(cloudOf(Kind.KindGCP)).toBe("gcp");
    expect(cloudOf(Kind.KindAWSSSORole)).toBe("aws");
    expect(cloudOf(Kind.KindAWSAssumeRole)).toBe("aws");
    expect(cloudOf(Kind.KindAWSIAMUser)).toBe("aws");
    expect(cloudOf(Kind.KindAWSLogin)).toBe("aws");
  });

  it("falls back to aws for unknown kinds", () => {
    expect(cloudOf("")).toBe("aws");
    expect(cloudOf("something-else")).toBe("aws");
  });
});

describe("kindLabel", () => {
  it("labels every session kind", () => {
    expect(kindLabel[Kind.KindAWSSSORole]).toBe("SSO role");
    expect(kindLabel[Kind.KindAWSAssumeRole]).toBe("Assume role");
    expect(kindLabel[Kind.KindAWSIAMUser]).toBe("IAM user");
    expect(kindLabel[Kind.KindAWSLogin]).toBe("Console login");
    expect(kindLabel[Kind.KindAzure]).toBe("Azure");
    expect(kindLabel[Kind.KindGCP]).toBe("GCP");
  });
});

describe("sessionSubtitle", () => {
  it("shows the subscription for Azure", () => {
    const s = session({ kind: Kind.KindAzure, azure: { subscriptionId: "sub-1", tenantId: "t-1" } });
    expect(sessionSubtitle(s)).toBe("sub-1");
  });

  it("shows the project, plus the service account when impersonating, for GCP", () => {
    expect(sessionSubtitle(session({ kind: Kind.KindGCP, gcp: { projectId: "proj" } }))).toBe("proj");
    expect(
      sessionSubtitle(session({ kind: Kind.KindGCP, gcp: { projectId: "proj", serviceAccount: "sa@proj.iam" } })),
    ).toBe("proj · sa@proj.iam");
  });

  it("shows account and role for SSO roles", () => {
    const s = session({ kind: Kind.KindAWSSSORole, aws: { accountId: "123456789012", roleName: "Admin" } });
    expect(sessionSubtitle(s)).toBe("123456789012 · Admin");
  });

  it("shows the role ARN for assume role", () => {
    const s = session({ kind: Kind.KindAWSAssumeRole, aws: { roleArn: "arn:aws:iam::1:role/x" } });
    expect(sessionSubtitle(s)).toBe("arn:aws:iam::1:role/x");
  });

  it("shows the account and region for a console login once signed in", () => {
    expect(
      sessionSubtitle(session({ kind: Kind.KindAWSLogin, region: "us-east-1", aws: { accountId: "123456789012" } })),
    ).toBe("123456789012 · us-east-1");
    expect(sessionSubtitle(session({ kind: Kind.KindAWSLogin, region: "us-east-1", aws: {} }))).toBe("us-east-1");
  });

  it("falls back to the region for IAM users and sessions without details", () => {
    expect(sessionSubtitle(session({ kind: Kind.KindAWSIAMUser, region: "us-west-2", aws: { mfaDevice: "" } }))).toBe(
      "us-west-2",
    );
    expect(sessionSubtitle(session({ kind: Kind.KindAWSIAMUser, region: "eu-west-1" }))).toBe("eu-west-1");
    expect(sessionSubtitle(session({ kind: Kind.KindAWSIAMUser, region: undefined }))).toBe("");
  });
});

describe("isLoggedIn", () => {
  it("treats an SSO token as a login only while it is in the future", () => {
    expect(
      isLoggedIn(
        integration({
          awsSso: { startUrl: "u", region: "r", tokenExpires: new Date(Date.now() + 60e3).toISOString() },
        }),
      ),
    ).toBe(true);
    expect(
      isLoggedIn(
        integration({
          // Past expiry with the field still set: the refresh token keeps the login alive.
          awsSso: { startUrl: "u", region: "r", tokenExpires: new Date(Date.now() - 60e3).toISOString() },
        }),
      ),
    ).toBe(true);
    expect(isLoggedIn(integration({ awsSso: { startUrl: "u", region: "r", tokenExpires: null } }))).toBe(false);
  });

  it("uses the signed-in account for Azure and GCP", () => {
    expect(isLoggedIn(integration({ cloud: Cloud.CloudAzure, azure: { tenantId: "t", account: "me@x" } }))).toBe(true);
    expect(isLoggedIn(integration({ cloud: Cloud.CloudAzure, azure: { tenantId: "t", account: "" } }))).toBe(false);
    expect(isLoggedIn(integration({ cloud: Cloud.CloudGCP, gcp: { account: "me@x" } }))).toBe(true);
    expect(isLoggedIn(integration({ cloud: Cloud.CloudGCP, gcp: { account: "" } }))).toBe(false);
  });

  it("is false without any provider block", () => {
    expect(isLoggedIn(integration({}))).toBe(false);
  });
});
