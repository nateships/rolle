import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { TooltipProvider } from "@/components/ui/tooltip";
import { SessionRow } from "@/components/dashboard/SessionRow";
import { Kind, Status, type Session, type Workspace } from "@/lib/api";
import { session, ssoRole } from "@/test/fixtures";

function renderRow(s: Session) {
  const ws = { version: 1, onboarded: true, sessions: [s], integrations: [] } as unknown as Workspace;
  return render(
    <TooltipProvider>
      <table>
        <tbody>
          <SessionRow session={s} workspace={ws} />
        </tbody>
      </table>
    </TooltipProvider>,
  );
}

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
});
