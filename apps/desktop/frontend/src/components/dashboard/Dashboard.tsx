import { useMemo, useState } from "react";
import { AnimatePresence, motion } from "motion/react";
import { Cloud, KeyRound, LogIn, LogOut, MoreHorizontal, Plus, RefreshCw, Search, ShieldCheck, Trash2, UserCog, Waypoints } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Lockup, CloudGlyph } from "@/components/Brand";
import { SessionRow } from "./SessionRow";
import { DevTools } from "@/components/DevTools";
import { AddSSODialog, AddAssumeRoleDialog, AddIAMUserDialog, AddAzureDialog, AddGCPDialog, AddGCPImpersonationDialog, LoginDialog } from "@/components/dialogs/Dialogs";
import { api, errorMessage, Cloud as CloudKind, Status, type Integration, type Workspace } from "@/lib/api";
import { cn } from "@/lib/utils";

type Dialog = null | { kind: "sso" } | { kind: "assume" } | { kind: "iam" } | { kind: "azure" } | { kind: "gcp" } | { kind: "gcp-impersonate" } | { kind: "login"; integration: Integration };

function isLoggedIn(integ: Integration): boolean {
  if (integ.awsSso) return !!integ.awsSso.tokenExpires && new Date(integ.awsSso.tokenExpires).getTime() > Date.now();
  if (integ.azure) return !!integ.azure.account;
  if (integ.gcp) return !!integ.gcp.account;
  return false;
}

const CLOUD_SECTIONS: { cloud: string; title: string; addKind: Dialog }[] = [
  { cloud: CloudKind.CloudAWS, title: "AWS Identity Center", addKind: { kind: "sso" } },
  { cloud: CloudKind.CloudAzure, title: "Azure tenants", addKind: { kind: "azure" } },
  { cloud: CloudKind.CloudGCP, title: "Google Cloud", addKind: { kind: "gcp" } },
];

export function Dashboard({ workspace }: { workspace: Workspace }) {
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<string | null>(null);
  const [dialog, setDialog] = useState<Dialog>(null);

  const sessions = useMemo(() => {
    const q = query.trim().toLowerCase();
    return workspace.sessions
      .filter((s) => (filter === null ? true : filter === "manual" ? !s.integrationId : s.integrationId === filter))
      .filter((s) => !q || s.name.toLowerCase().includes(q) || (s.aws?.accountId ?? "").includes(q) || (s.aws?.roleName ?? "").toLowerCase().includes(q))
      .sort((a, b) => Number(b.status === Status.StatusActive) - Number(a.status === Status.StatusActive) || a.name.localeCompare(b.name));
  }, [workspace.sessions, query, filter]);

  const active = workspace.sessions.filter((s) => s.status === Status.StatusActive).length;
  const manualCount = workspace.sessions.filter((s) => !s.integrationId).length;

  async function run(label: string, fn: () => Promise<unknown>) {
    try {
      await fn();
      toast.success(label);
    } catch (e) {
      toast.error(errorMessage(e));
    }
  }

  return (
    <div className="flex h-full">
      <aside className="flex w-64 shrink-0 flex-col border-r bg-sidebar text-sidebar-foreground">
        <div className="drag flex h-14 items-center justify-end px-4 pt-2">
          <Lockup className="h-7" markClassName="size-7" />
        </div>
        <nav className="flex-1 space-y-5 overflow-y-auto px-3 py-2">
          <div>
            <SideItem active={filter === null} onClick={() => setFilter(null)} label="All sessions" count={workspace.sessions.length} />
            {manualCount > 0 && <SideItem active={filter === "manual"} onClick={() => setFilter("manual")} label="Manual" count={manualCount} />}
          </div>
          {CLOUD_SECTIONS.map((sec) => {
            const items = workspace.integrations.filter((i) => i.cloud === sec.cloud);
            return (
              <div key={sec.cloud}>
                <div className="mb-1 flex items-center justify-between px-2">
                  <p className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground">{sec.title}</p>
                  <Button variant="ghost" size="icon-xs" onClick={() => setDialog(sec.addKind)} title="Add"><Plus className="size-3.5" /></Button>
                </div>
                {items.length === 0 && <p className="px-2 py-1 text-xs text-muted-foreground/70">None yet.</p>}
                {items.map((integ) => {
                  const loggedIn = isLoggedIn(integ);
                  const sync = integ.cloud === CloudKind.CloudAzure ? api.SyncAzure : integ.cloud === CloudKind.CloudGCP ? api.SyncGCP : api.SyncSSO;
                  const logout = integ.cloud === CloudKind.CloudAzure ? api.AzureLogout : api.SSOLogout;
                  return (
                    <div key={integ.id} className="group flex items-center">
                      <SideItem
                        active={filter === integ.id}
                        onClick={() => setFilter(integ.id)}
                        label={integ.alias}
                        dot={loggedIn ? "ok" : "off"}
                        count={workspace.sessions.filter((s) => s.integrationId === integ.id).length}
                      />
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button variant="ghost" size="icon-xs" className="opacity-0 group-hover:opacity-100 data-[state=open]:opacity-100"><MoreHorizontal className="size-3.5" /></Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="start" side="right">
                          {loggedIn ? (
                            <>
                              <DropdownMenuItem onClick={() => run("Synced", () => sync(integ.id))}><RefreshCw /> Sync</DropdownMenuItem>
                              {integ.cloud !== CloudKind.CloudGCP && <DropdownMenuItem onClick={() => run("Signed out", () => logout(integ.id))}><LogOut /> Sign out</DropdownMenuItem>}
                            </>
                          ) : (
                            <DropdownMenuItem onClick={() => (integ.cloud === CloudKind.CloudGCP ? run("Synced", () => api.SyncGCP(integ.id)) : setDialog({ kind: "login", integration: integ }))}><LogIn /> Sign in</DropdownMenuItem>
                          )}
                          {integ.cloud === CloudKind.CloudGCP && <DropdownMenuItem onClick={() => setDialog({ kind: "gcp-impersonate" })}><UserCog /> Impersonate service account</DropdownMenuItem>}
                          <DropdownMenuSeparator />
                          <DropdownMenuItem variant="destructive" onClick={() => run("Removed", () => api.RemoveIntegration(integ.id))}><Trash2 /> Remove</DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </div>
                  );
                })}
              </div>
            );
          })}
        </nav>
        <div className="flex items-center justify-between border-t p-3 text-xs text-muted-foreground">
          <div className="flex items-center gap-2">
            <span className={cn("relative inline-block size-2 rounded-full", active > 0 ? "bg-emerald-400 text-emerald-400 pulse-ring" : "bg-muted-foreground/40")} />
            {active} active
          </div>
          <DevTools />
        </div>
      </aside>

      <section className="flex min-w-0 flex-1 flex-col">
        <header className="drag flex h-14 items-center gap-3 border-b px-5 pt-2">
          <div className="no-drag relative max-w-sm flex-1">
            <Search className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search sessions" className="h-8 pl-8" />
          </div>
          <div className="flex-1" />
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button size="sm" className="no-drag gap-1.5"><Plus className="size-4" /> Add</Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onClick={() => setDialog({ kind: "sso" })}><ShieldCheck /> AWS Identity Center portal</DropdownMenuItem>
              <DropdownMenuItem onClick={() => setDialog({ kind: "assume" })} disabled={!workspace.sessions.some((s) => s.aws)}><Waypoints /> AWS assume role</DropdownMenuItem>
              <DropdownMenuItem onClick={() => setDialog({ kind: "iam" })}><KeyRound /> AWS IAM user access key</DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={() => setDialog({ kind: "azure" })}><Cloud /> Azure tenant</DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={() => setDialog({ kind: "gcp" })}><Cloud /> Google Cloud account</DropdownMenuItem>
              <DropdownMenuItem onClick={() => setDialog({ kind: "gcp-impersonate" })} disabled={!workspace.integrations.some((i) => i.gcp)}><UserCog /> GCP service account impersonation</DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </header>

        <div className="flex-1 overflow-y-auto p-4">
          {sessions.length === 0 ? (
            <Empty hasAny={workspace.sessions.length > 0} onAdd={() => setDialog({ kind: "sso" })} />
          ) : (
            <motion.ul layout className="space-y-1.5">
              <AnimatePresence initial={false}>
                {sessions.map((s) => (
                  <SessionRow key={s.id} session={s} workspace={workspace} />
                ))}
              </AnimatePresence>
            </motion.ul>
          )}
        </div>
      </section>

      <AddSSODialog open={dialog?.kind === "sso"} onClose={() => setDialog(null)} onLogin={(integ) => setDialog({ kind: "login", integration: integ })} />
      <AddAssumeRoleDialog open={dialog?.kind === "assume"} onClose={() => setDialog(null)} workspace={workspace} />
      <AddIAMUserDialog open={dialog?.kind === "iam"} onClose={() => setDialog(null)} />
      <AddAzureDialog open={dialog?.kind === "azure"} onClose={() => setDialog(null)} onLogin={(integ) => setDialog({ kind: "login", integration: integ })} />
      <AddGCPDialog open={dialog?.kind === "gcp"} onClose={() => setDialog(null)} />
      <AddGCPImpersonationDialog open={dialog?.kind === "gcp-impersonate"} onClose={() => setDialog(null)} workspace={workspace} />
      <LoginDialog integration={dialog?.kind === "login" ? dialog.integration : null} onClose={() => setDialog(null)} />
    </div>
  );
}

function SideItem({ active, onClick, label, count, dot }: { active: boolean; onClick: () => void; label: string; count?: number; dot?: "ok" | "off" }) {
  return (
    <button type="button" onClick={onClick} className={cn("flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors", active ? "bg-sidebar-accent text-sidebar-accent-foreground" : "text-muted-foreground hover:bg-sidebar-accent/60 hover:text-foreground")}>
      {dot && <span className={cn("size-1.5 rounded-full", dot === "ok" ? "bg-emerald-400" : "bg-muted-foreground/40")} />}
      <span className="flex-1 truncate text-left">{label}</span>
      {count !== undefined && <span className="text-xs tabular-nums text-muted-foreground/70">{count}</span>}
    </button>
  );
}

function Empty({ hasAny, onAdd }: { hasAny: boolean; onAdd: () => void }) {
  return (
    <div className="flex h-full flex-col items-center justify-center text-center">
      <CloudGlyph cloud="aws" className="size-12 p-2" />
      <h3 className="mt-5 text-lg font-medium">{hasAny ? "Nothing matches" : "No sessions yet"}</h3>
      <p className="mt-1 max-w-sm text-sm text-muted-foreground">{hasAny ? "Try a different search or filter." : "Connect an Identity Center portal to discover every role you can reach."}</p>
      {!hasAny && <Button className="mt-5 gap-1.5" onClick={onAdd}><Plus className="size-4" /> Add portal</Button>}
    </div>
  );
}
