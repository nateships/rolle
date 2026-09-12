import { useEffect, useState } from "react";
import { Kind, type Integration, type Session } from "./api";

/** Whether an integration currently holds a usable sign-in. */
export function isLoggedIn(integ: Integration): boolean {
  // The backend clears tokenExpires when only a new login can help. A time in
  // the past means the access token lapsed but a refresh token can renew it.
  if (integ.awsSso) return !!integ.awsSso.tokenExpires;
  if (integ.azure) return !!integ.azure.account;
  if (integ.gcp) return !!integ.gcp.account;
  return false;
}

export const kindLabel: Record<string, string> = {
  [Kind.KindAWSSSORole]: "SSO role",
  [Kind.KindAWSAssumeRole]: "Assume role",
  [Kind.KindAWSIAMUser]: "IAM user",
  [Kind.KindAzure]: "Azure",
  [Kind.KindGCP]: "GCP",
};

export function cloudOf(kind: string): "aws" | "azure" | "gcp" {
  if (kind === Kind.KindAzure) return "azure";
  if (kind === Kind.KindGCP) return "gcp";
  return "aws";
}

/** Re-render every second so countdowns tick. */
export function useNow(intervalMs = 1000) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), intervalMs);
    return () => clearInterval(id);
  }, [intervalMs]);
  return now;
}

export function remaining(expires: string | null | undefined, now: number): string {
  if (!expires) return "";
  const ms = new Date(expires).getTime() - now;
  if (ms <= 0) return "expired";
  const total = Math.floor(ms / 1000);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m ${s.toString().padStart(2, "0")}s`;
  return `${s}s`;
}

export function sessionSubtitle(s: Session): string {
  if (s.azure) return s.azure.subscriptionId;
  if (s.gcp) return s.gcp.serviceAccount ? `${s.gcp.projectId} · ${s.gcp.serviceAccount}` : s.gcp.projectId;
  const a = s.aws;
  if (!a) return s.region ?? "";
  if (s.kind === Kind.KindAWSSSORole) return `${a.accountId ?? ""} · ${a.roleName ?? ""}`;
  if (s.kind === Kind.KindAWSAssumeRole) return a.roleArn ?? "";
  return s.region ?? "";
}
