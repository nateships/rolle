import { useEffect, useState } from "react";
import { ChevronDown, Import, Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CloudGlyph } from "@/components/Brand";
import { RemoveKeysDialog, type RemoveKeysTarget } from "@/components/dialogs/RemoveKeysDialog";
import { api, errorMessage, type Workspace } from "@/lib/api";
import { cn } from "@/lib/utils";

export type FoundPortal = {
  alias: string;
  startUrl: string;
  region: string;
  profiles: string[];
  hasToken: boolean;
  source: string;
};
export type FoundTenant = { tenantId: string; account: string; source: string };
export type FoundLeapp = { iamUsers: { name: string }[]; chainedRoles: { name: string }[]; ssoRoles: number };
/** An access key in ~/.aws/credentials. The secret stays in the file. */
export type FoundIAMUser = {
  profile: string;
  accessKeyId: string;
  region: string;
  mfaDevice: string;
  imported: boolean;
};
export type Found = {
  awsPortals: FoundPortal[];
  azureTenants: FoundTenant[];
  iamUsers: FoundIAMUser[];
  gcp: { account: string } | null;
  leapp: FoundLeapp | null;
};

const EMPTY: Found = { awsPortals: [], azureTenants: [], iamUsers: [], gcp: null, leapp: null };

const SOURCE_LABEL: Record<string, string> = { "aws-cli": "AWS CLI", granted: "Granted", leapp: "Leapp", az: "az CLI" };

/** Scan the machine once and filter out identities the workspace already has. */
export function useDiscovery(workspace: Workspace, enabled = true) {
  const [found, setFound] = useState<Found | null>(null);
  useEffect(() => {
    if (!enabled || found) return;
    // Go nil slices arrive as null; normalise every list before anything calls .filter or .length.
    api
      .Discover()
      .then((r) => {
        const raw = (r ?? {}) as Partial<{
          awsPortals: Partial<FoundPortal>[] | null;
          azureTenants: Partial<FoundTenant>[] | null;
          iamUsers: Partial<FoundIAMUser>[] | null;
          gcp: { account: string } | null;
          leapp: Partial<{
            iamUsers: { name: string }[] | null;
            chainedRoles: { name: string }[] | null;
            ssoRoles: number;
          }> | null;
        }>;
        setFound({
          awsPortals: (raw.awsPortals ?? []).map((p) => ({
            alias: p.alias ?? "aws",
            startUrl: p.startUrl ?? "",
            region: p.region ?? "us-east-1",
            profiles: p.profiles ?? [],
            hasToken: !!p.hasToken,
            source: p.source ?? "aws-cli",
          })),
          azureTenants: (raw.azureTenants ?? []).map((t) => ({
            tenantId: t.tenantId ?? "",
            account: t.account ?? "",
            source: t.source ?? "az",
          })),
          iamUsers: (raw.iamUsers ?? []).map((u) => ({
            profile: u.profile ?? "",
            accessKeyId: u.accessKeyId ?? "",
            region: u.region ?? "",
            mfaDevice: u.mfaDevice ?? "",
            imported: !!u.imported,
          })),
          gcp: raw.gcp ?? null,
          leapp: raw.leapp
            ? {
                iamUsers: raw.leapp.iamUsers ?? [],
                chainedRoles: raw.leapp.chainedRoles ?? [],
                ssoRoles: raw.leapp.ssoRoles ?? 0,
              }
            : null,
        });
      })
      .catch((e) => {
        // A failed scan is not an empty one; say so instead of "nothing new".
        toast.error("Scan failed", { description: errorMessage(e) });
        setFound(EMPTY);
      });
  }, [enabled, found]);

  const trim = (u: string) => u.replace(/\/$/, "");
  const portals = (found?.awsPortals ?? []).filter(
    (p) => !workspace.integrations.some((i) => i.awsSso?.startUrl && trim(i.awsSso.startUrl) === trim(p.startUrl)),
  );
  const tenants = (found?.azureTenants ?? []).filter(
    (t) => !workspace.integrations.some((i) => i.azure?.tenantId === t.tenantId),
  );
  const gcp = found?.gcp && !workspace.integrations.some((i) => i.gcp) ? found.gcp : null;
  // Keys a session already holds, and profiles whose name a session took, are not offered.
  const iamUsers = (found?.iamUsers ?? []).filter(
    (u) => !u.imported && !workspace.sessions.some((s) => s.name === u.profile),
  );
  // Leapp sessions worth importing: users and chained roles not already present by name.
  const leapp =
    found?.leapp &&
    [...found.leapp.iamUsers, ...found.leapp.chainedRoles].some(
      (x) => !workspace.sessions.some((s) => s.name === x.name),
    )
      ? found.leapp
      : null;
  return {
    loading: found === null,
    portals,
    tenants,
    gcp,
    leapp,
    iamUsers,
    count: portals.length + tenants.length + iamUsers.length + (gcp ? 1 : 0) + (leapp ? 1 : 0),
    rescan: () => setFound(null),
  };
}

export function FoundList({
  portals,
  tenants,
  gcp,
  leapp,
  iamUsers = [],
  importing,
  disabled,
  onAWS,
  onAzure,
  onGCP,
  onLeapp,
}: {
  portals: FoundPortal[];
  tenants: FoundTenant[];
  gcp: { account: string } | null;
  leapp?: FoundLeapp | null;
  iamUsers?: FoundIAMUser[];
  importing: string | null;
  disabled: boolean;
  onAWS: (p: FoundPortal) => void;
  onAzure: (t: FoundTenant) => void;
  onGCP: () => void;
  onLeapp?: () => void;
}) {
  const leappCount = leapp ? leapp.iamUsers.length + leapp.chainedRoles.length : 0;
  // An IAM user imports here: the key moves into rolle, then the dialog offers to remove it from the file.
  const [importingUser, setImportingUser] = useState<string | null>(null);
  // The IAM users start folded, so a long credentials file leaves room for the rest of the step.
  const [usersOpen, setUsersOpen] = useState(false);
  const [removing, setRemoving] = useState<RemoveKeysTarget | null>(null);
  // importUsers moves each key into rolle in turn, then offers to remove
  // the imported ones from the file together. "*" marks an import of all.
  async function importUsers(users: FoundIAMUser[]) {
    setImportingUser(users.length === 1 ? users[0].profile : "*");
    const done: string[] = [];
    try {
      for (const u of users) {
        await api.ImportIAMUser(u.profile);
        done.push(u.profile);
      }
      toast.success("Imported", { description: done.length === 1 ? done[0] : `${done.length} IAM users` });
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setImportingUser(null);
    }
    if (done.length > 0) setRemoving({ profiles: done });
  }
  return (
    // The list scrolls inside a cap, so a long credentials file does not
    // push the rest of the step off the screen.
    <ul className="space-y-2">
      {portals.map((p) => (
        <FoundRow
          key={p.startUrl}
          cloud="aws"
          title={p.alias}
          subtitle={`${p.startUrl} · ${p.region}${p.profiles.length ? ` · ${p.profiles.length} profile${p.profiles.length === 1 ? "" : "s"}` : ""}`}
          badge={p.hasToken ? "Signed in via AWS CLI" : `From ${SOURCE_LABEL[p.source] ?? p.source}`}
          badgeOk={p.hasToken}
          busy={importing === p.startUrl}
          disabled={disabled}
          onImport={() => onAWS(p)}
        />
      ))}
      {tenants.map((t) => (
        <FoundRow
          key={t.tenantId}
          cloud="azure"
          title={t.account || "Azure tenant"}
          subtitle={t.tenantId}
          badge={`From ${SOURCE_LABEL[t.source] ?? t.source}`}
          badgeOk
          busy={importing === t.tenantId}
          disabled={disabled}
          onImport={() => onAzure(t)}
        />
      ))}
      {gcp && (
        <FoundRow
          cloud="gcp"
          title={gcp.account || "Google account"}
          subtitle="gcloud Application Default Credentials"
          badge="Signed in via gcloud"
          badgeOk
          busy={importing === "gcp"}
          disabled={disabled}
          onImport={onGCP}
        />
      )}
      {leapp && onLeapp && leappCount > 0 && (
        <FoundRow
          cloud="aws"
          title={`${leappCount} Leapp session${leappCount === 1 ? "" : "s"}`}
          subtitle={[
            leapp.iamUsers.length ? `${leapp.iamUsers.length} IAM user${leapp.iamUsers.length === 1 ? "" : "s"}` : "",
            leapp.chainedRoles.length
              ? `${leapp.chainedRoles.length} chained role${leapp.chainedRoles.length === 1 ? "" : "s"}`
              : "",
            leapp.ssoRoles ? `${leapp.ssoRoles} SSO roles return when you sync the portal` : "",
          ]
            .filter(Boolean)
            .join(" · ")}
          badge="From Leapp"
          badgeOk
          busy={importing === "leapp"}
          disabled={disabled}
          onImport={onLeapp}
        />
      )}
      {iamUsers.length > 0 && (
        <li className="flex items-center justify-between gap-3 pt-1 text-xs text-muted-foreground">
          <button
            type="button"
            className="flex items-center gap-1.5 rounded hover:text-foreground"
            aria-expanded={usersOpen}
            onClick={() => setUsersOpen((o) => !o)}
          >
            <ChevronDown className={cn("size-3.5 transition-transform", !usersOpen && "-rotate-90")} />
            {iamUsers.length} IAM user{iamUsers.length === 1 ? "" : "s"} in the credentials file
          </button>
          <Button
            size="sm"
            variant="ghost"
            className="h-6 shrink-0 gap-1.5 text-xs"
            disabled={disabled || importingUser !== null}
            onClick={() => void importUsers(iamUsers)}
          >
            {importingUser === "*" ? <Loader2 className="size-3 animate-spin" /> : <Import className="size-3" />}
            {iamUsers.length === 1 ? "Import" : "Import all"}
          </Button>
        </li>
      )}
      {usersOpen &&
        iamUsers.map((u) => (
          <FoundRow
            key={u.profile}
            cloud="aws"
            title={u.profile}
            subtitle={[`${u.accessKeyId.slice(0, 4)}…`, u.region].filter(Boolean).join(" · ")}
            badge={u.mfaDevice ? "MFA" : undefined}
            badgeOk
            compact
            busy={importingUser === u.profile || importingUser === "*"}
            disabled={disabled || importingUser !== null}
            onImport={() => void importUsers([u])}
          />
        ))}
      <RemoveKeysDialog target={removing} onClose={() => setRemoving(null)} />
    </ul>
  );
}

function FoundRow({
  cloud,
  title,
  subtitle,
  badge,
  badgeOk,
  compact,
  busy,
  disabled,
  onImport,
}: {
  cloud: "aws" | "azure" | "gcp";
  title: string;
  subtitle: string;
  badge?: string;
  badgeOk?: boolean;
  /** One line: the subtitle sits after the title, and the row is shorter. */
  compact?: boolean;
  busy: boolean;
  disabled: boolean;
  onImport: () => void;
}) {
  return (
    <li
      className={cn("flex items-center gap-3 rounded-lg border bg-background/60 px-3", compact ? "py-1.5" : "py-2.5")}
    >
      <CloudGlyph cloud={cloud} className={compact ? "size-6" : undefined} />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <p className="truncate text-sm font-medium">{title}</p>
          {badge && (
            <Badge
              variant="outline"
              className={cn(
                "h-5 px-1.5 text-[10px] font-normal",
                badgeOk ? "border-brand-green/40 bg-brand-green/10 text-brand-green-text" : "text-muted-foreground",
              )}
            >
              {badge}
            </Badge>
          )}
          {compact && subtitle && <p className="truncate font-mono text-[11px] text-muted-foreground">{subtitle}</p>}
        </div>
        {!compact && <p className="truncate font-mono text-[11px] text-muted-foreground">{subtitle}</p>}
      </div>
      <Button
        size="sm"
        variant="secondary"
        className={cn("gap-1.5", compact && "h-7")}
        onClick={onImport}
        disabled={disabled}
      >
        {busy ? <Loader2 className="size-3.5 animate-spin" /> : <Import className="size-3.5" />} Import
      </Button>
    </li>
  );
}

/** Derive an alias for an Azure tenant: the account's domain, or the name another tool gave it. */
export function tenantAlias(t: FoundTenant): string {
  if (t.account.includes("@")) return t.account.split("@")[1].split(".")[0];
  return t.account || "azure";
}
