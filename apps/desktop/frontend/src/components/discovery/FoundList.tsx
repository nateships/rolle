import { useEffect, useState } from "react";
import { Import, Loader2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CloudGlyph } from "@/components/Brand";
import { api, type Workspace } from "@/lib/api";
import { cn } from "@/lib/utils";

export type FoundPortal = { alias: string; startUrl: string; region: string; profiles: string[]; hasToken: boolean; source: string };
export type FoundTenant = { tenantId: string; account: string; source: string };
export type FoundLeapp = { iamUsers: { name: string }[]; chainedRoles: { name: string }[]; ssoRoles: number };
export type Found = { awsPortals: FoundPortal[]; azureTenants: FoundTenant[]; gcp: { account: string } | null; leapp: FoundLeapp | null };

const EMPTY: Found = { awsPortals: [], azureTenants: [], gcp: null, leapp: null };

const SOURCE_LABEL: Record<string, string> = { "aws-cli": "AWS CLI", granted: "Granted", leapp: "Leapp", az: "az CLI" };

/** Scan the machine once and filter out identities the workspace already has. */
export function useDiscovery(workspace: Workspace, enabled = true) {
  const [found, setFound] = useState<Found | null>(null);
  useEffect(() => {
    if (!enabled || found) return;
    // Go nil slices arrive as null; normalise every list before anything calls .filter or .length.
    api.Discover()
      .then((r) => {
        const raw = (r ?? {}) as Partial<{ awsPortals: Partial<FoundPortal>[] | null; azureTenants: Partial<FoundTenant>[] | null; gcp: { account: string } | null; leapp: Partial<{ iamUsers: { name: string }[] | null; chainedRoles: { name: string }[] | null; ssoRoles: number }> | null }>;
        setFound({
          awsPortals: (raw.awsPortals ?? []).map((p) => ({ alias: p.alias ?? "aws", startUrl: p.startUrl ?? "", region: p.region ?? "us-east-1", profiles: p.profiles ?? [], hasToken: !!p.hasToken, source: p.source ?? "aws-cli" })),
          azureTenants: (raw.azureTenants ?? []).map((t) => ({ tenantId: t.tenantId ?? "", account: t.account ?? "", source: t.source ?? "az" })),
          gcp: raw.gcp ?? null,
          leapp: raw.leapp ? { iamUsers: raw.leapp.iamUsers ?? [], chainedRoles: raw.leapp.chainedRoles ?? [], ssoRoles: raw.leapp.ssoRoles ?? 0 } : null,
        });
      })
      .catch(() => setFound(EMPTY));
  }, [enabled, found]);

  const trim = (u: string) => u.replace(/\/$/, "");
  const portals = (found?.awsPortals ?? []).filter((p) => !workspace.integrations.some((i) => i.awsSso?.startUrl && trim(i.awsSso.startUrl) === trim(p.startUrl)));
  const tenants = (found?.azureTenants ?? []).filter((t) => !workspace.integrations.some((i) => i.azure?.tenantId === t.tenantId));
  const gcp = found?.gcp && !workspace.integrations.some((i) => i.gcp) ? found.gcp : null;
  // Leapp sessions worth importing: users and chained roles not already present by name.
  const leapp = found?.leapp && [...found.leapp.iamUsers, ...found.leapp.chainedRoles].some((x) => !workspace.sessions.some((s) => s.name === x.name)) ? found.leapp : null;
  return { loading: found === null, portals, tenants, gcp, leapp, count: portals.length + tenants.length + (gcp ? 1 : 0) + (leapp ? 1 : 0), rescan: () => setFound(null) };
}

export function FoundList({ portals, tenants, gcp, leapp, importing, disabled, onAWS, onAzure, onGCP, onLeapp }: {
  portals: FoundPortal[]; tenants: FoundTenant[]; gcp: { account: string } | null; leapp?: FoundLeapp | null;
  importing: string | null; disabled: boolean;
  onAWS: (p: FoundPortal) => void; onAzure: (t: FoundTenant) => void; onGCP: () => void; onLeapp?: () => void;
}) {
  const leappCount = leapp ? leapp.iamUsers.length + leapp.chainedRoles.length : 0;
  return (
    <ul className="space-y-2">
      {portals.map((p) => (
        <FoundRow key={p.startUrl} cloud="aws" title={p.alias} subtitle={`${p.startUrl} · ${p.region}${p.profiles.length ? ` · ${p.profiles.length} profile${p.profiles.length === 1 ? "" : "s"}` : ""}`} badge={p.hasToken ? "Signed in via AWS CLI" : `From ${SOURCE_LABEL[p.source] ?? p.source}`} badgeOk={p.hasToken} busy={importing === p.startUrl} disabled={disabled} onImport={() => onAWS(p)} />
      ))}
      {tenants.map((t) => (
        <FoundRow key={t.tenantId} cloud="azure" title={t.account || "Azure tenant"} subtitle={t.tenantId} badge={`From ${SOURCE_LABEL[t.source] ?? t.source}`} badgeOk busy={importing === t.tenantId} disabled={disabled} onImport={() => onAzure(t)} />
      ))}
      {gcp && <FoundRow cloud="gcp" title={gcp.account || "Google account"} subtitle="gcloud Application Default Credentials" badge="Signed in via gcloud" badgeOk busy={importing === "gcp"} disabled={disabled} onImport={onGCP} />}
      {leapp && onLeapp && leappCount > 0 && (
        <FoundRow cloud="aws" title={`${leappCount} Leapp session${leappCount === 1 ? "" : "s"}`} subtitle={[leapp.iamUsers.length ? `${leapp.iamUsers.length} IAM user${leapp.iamUsers.length === 1 ? "" : "s"}` : "", leapp.chainedRoles.length ? `${leapp.chainedRoles.length} chained role${leapp.chainedRoles.length === 1 ? "" : "s"}` : "", leapp.ssoRoles ? `${leapp.ssoRoles} SSO roles return when you sync the portal` : ""].filter(Boolean).join(" · ")} badge="From Leapp" badgeOk busy={importing === "leapp"} disabled={disabled} onImport={onLeapp} />
      )}
    </ul>
  );
}

function FoundRow({ cloud, title, subtitle, badge, badgeOk, busy, disabled, onImport }: { cloud: "aws" | "azure" | "gcp"; title: string; subtitle: string; badge: string; badgeOk?: boolean; busy: boolean; disabled: boolean; onImport: () => void }) {
  return (
    <li className="flex items-center gap-3 rounded-lg border bg-background/60 px-3 py-2.5">
      <CloudGlyph cloud={cloud} />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <p className="truncate text-sm font-medium">{title}</p>
          <Badge variant="outline" className={cn("h-5 px-1.5 text-[10px] font-normal", badgeOk ? "border-brand-green/40 text-brand-green" : "text-muted-foreground")}>{badge}</Badge>
        </div>
        <p className="truncate font-mono text-[11px] text-muted-foreground">{subtitle}</p>
      </div>
      <Button size="sm" variant="secondary" className="gap-1.5" onClick={onImport} disabled={disabled}>
        {busy ? <Loader2 className="size-3.5 animate-spin" /> : <Import className="size-3.5" />} Import
      </Button>
    </li>
  );
}

/** Derive an alias for an Azure tenant from the signed-in account. */
export function tenantAlias(t: FoundTenant): string {
  return t.account.includes("@") ? t.account.split("@")[1].split(".")[0] : "azure";
}
