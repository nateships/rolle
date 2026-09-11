import { Kind, type Integration, type Session } from "@/lib/api";

let n = 0;

/** Build a session with defaults. Callers override any field. */
export function session(o: Partial<Session>): Session {
  n += 1;
  return {
    id: `s${n}`,
    name: `session-${n}`,
    kind: "aws-iam-user",
    status: "inactive",
    region: "us-east-1",
    ...o,
  } as unknown as Session;
}

export function integration(o: Partial<Integration>): Integration {
  n += 1;
  return { id: `i${n}`, alias: `integ-${n}`, cloud: "aws", ...o } as unknown as Integration;
}

/** An SSO role named the way the backend names them: "<account>/<role>". */
export function ssoRole(account: string, accountId: string, role: string, extra: Partial<Session> = {}): Session {
  return session({
    name: `${account}/${role}`,
    kind: Kind.KindAWSSSORole,
    integrationId: "acme",
    aws: { accountId, roleName: role },
    ...extra,
  });
}
