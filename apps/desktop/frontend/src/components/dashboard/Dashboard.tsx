import { useMemo, useState } from "react";
import { AnimatePresence, motion } from "motion/react";
import { Cloud, Import, KeyRound, LogIn, LogOut, MoreHorizontal, Pencil, Plus, RefreshCw, Search, Settings as SettingsIcon, ShieldCheck, Star, Trash2, UserCog, Waypoints } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Lockup, Mark } from "@/components/Brand";
import { SessionRow } from "./SessionRow";
import { DevTools } from "@/components/DevTools";
import { SettingsDialog } from "@/components/dialogs/SettingsDialog";
import { RenameDialog } from "@/components/dialogs/RenameDialog";
import { ImportDialog } from "@/components/dialogs/ImportDialog";
import { inWails } from "@/lib/api";
import { AddSSODialog, AddAssumeRoleDialog, AddIAMUserDialog, AddAzureDialog, AddGCPDialog, AddGCPImpersonationDialog, LoginDialog } from "@/components/dialogs/Dialogs";
import { api, errorMessage, Cloud as CloudKind, Status, type Integration, type Workspace } from "@/lib/api";
import { cn } from "@/lib/utils";

type Dialog = null | { kind: "sso" } | { kind: "assume" } | { kind: "iam" } | { kind: "azure" } | { kind: "gcp" } | { kind: "gcp-impersonate" } | { kind: "login"; integration: Integration } | { kind: "settings" } | { kind: "rename"; integration: Integration } | { kind: "import" };

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
  // ?settings=1 opens the settings dialog in the browser preview.
  const [dialog, setDialog] = useState<Dialog>(() => {
    if (inWails) return null;
    const q = new URLSearchParams(location.search);
    return q.get("settings") ? { kind: "settings" } : q.get("import") ? { kind: "import" } : null;
  });

  const sessions = useMemo(() => {
    const q = query.trim().toLowerCase();
    return workspace.sessions
      .filter((s) => (filter === null ? true : filter === "manual" ? !s.integrationId : filter === "favorites" ? s.favorite : s.integrationId === filter))
      .filter((s) => !q || s.name.toLowerCase().includes(q) || (s.aws?.accountId ?? "").includes(q) || (s.aws?.roleName ?? "").toLowerCase().includes(q))
      .sort((a, b) => Number(b.status === Status.StatusActive) - Number(a.status === Status.StatusActive) || a.name.localeCompare(b.name));
  }, [workspace.sessions, query, filter]);

  const active = workspace.sessions.filter((s) => s.status === Status.StatusActive).length;
  const manualCount = workspace.sessions.filter((s) => !s.integrationId).length;
  const favorites = sessions.filter((s) => s.favorite);
  // Favorites get their own panel on the unfiltered list; elsewhere they sit inline.
  const showFavoritesPanel = filter === null && favorites.length > 0;
  const rest = showFavoritesPanel ? sessions.filter((s) => !s.favorite) : sessions;

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
        <nav className="flex-1 space-y-4 overflow-y-auto px-2 py-2">
          <div>
            <SideItem active={filter === null} onClick={() => setFilter(null)} label="All sessions" count={workspace.sessions.length} />
            {workspace.sessions.some((s) => s.favorite) && <SideItem active={filter === "favorites"} onClick={() => setFilter("favorites")} label="Favorites" count={workspace.sessions.filter((s) => s.favorite).length} icon={<Star className="size-3.5 fill-current text-brand-orange" />} />}
            {manualCount > 0 && <SideItem active={filter === "manual"} onClick={() => setFilter("manual")} label="Manual" count={manualCount} />}
          </div>
          {CLOUD_SECTIONS.map((sec) => {
            const items = workspace.integrations.filter((i) => i.cloud === sec.cloud);
            return (
              <div key={sec.cloud}>
                <div className="flex items-center pr-1">
                  <p className="flex-1 px-2 py-1 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">{sec.title}</p>
                  <span className="flex size-6 items-center justify-center">
                    <Button variant="ghost" size="icon-xs" className="text-muted-foreground" onClick={() => setDialog(sec.addKind)} title="Add"><Plus className="size-3.5" /></Button>
                  </span>
                </div>
                {items.length === 0 && <p className="px-2 py-1 text-xs text-muted-foreground/60">None yet</p>}
                {items.map((integ) => {
                  const loggedIn = isLoggedIn(integ);
                  const sync = integ.cloud === CloudKind.CloudAzure ? api.SyncAzure : integ.cloud === CloudKind.CloudGCP ? api.SyncGCP : api.SyncSSO;
                  const logout = integ.cloud === CloudKind.CloudAzure ? api.AzureLogout : api.SSOLogout;
                  return (
                    <SideItem
                      key={integ.id}
                      active={filter === integ.id}
                      onClick={() => setFilter(integ.id)}
                      label={integ.alias}
                      dot={loggedIn ? "ok" : "off"}
                      count={workspace.sessions.filter((s) => s.integrationId === integ.id).length}
                      trailing={
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button variant="ghost" size="icon-xs" className="text-muted-foreground opacity-0 group-hover:opacity-100 data-[state=open]:opacity-100" aria-label={`${integ.alias} options`}><MoreHorizontal className="size-3.5" /></Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="start" side="right" className="min-w-56">
                            <DropdownMenuItem onClick={() => setDialog({ kind: "rename", integration: integ })}><Pencil /> Rename</DropdownMenuItem>
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
                      }
                    />
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
          <div className="flex items-center gap-1">
            <DevTools />
            <Button variant="ghost" size="icon-sm" aria-label="Settings" onClick={() => setDialog({ kind: "settings" })}><SettingsIcon className="size-4" /></Button>
          </div>
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
            <DropdownMenuContent align="end" className="min-w-72">
              <DropdownMenuItem onClick={() => setDialog({ kind: "import" })}><Import /> Import from this machine</DropdownMenuItem>
              <DropdownMenuSeparator />
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
            <Empty hasAny={workspace.sessions.length > 0} onImport={() => setDialog({ kind: "import" })} onConnect={() => setDialog({ kind: "sso" })} />
          ) : (
            <div className="space-y-5">
              {showFavoritesPanel && (
                <section>
                  <h2 className="mb-2 flex items-center gap-1.5 px-1 text-[11px] font-medium uppercase tracking-wider text-muted-foreground"><Star className="size-3 fill-current text-brand-orange" /> Favorites</h2>
                  <motion.ul layout className="space-y-1.5">
                    <AnimatePresence initial={false}>
                      {favorites.map((s) => <SessionRow key={s.id} session={s} workspace={workspace} />)}
                    </AnimatePresence>
                  </motion.ul>
                </section>
              )}
              <section>
                {showFavoritesPanel && <h2 className="mb-2 px-1 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">All sessions</h2>}
                <motion.ul layout className="space-y-1.5">
                  <AnimatePresence initial={false}>
                    {rest.map((s) => <SessionRow key={s.id} session={s} workspace={workspace} />)}
                  </AnimatePresence>
                </motion.ul>
              </section>
            </div>
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
      <SettingsDialog open={dialog?.kind === "settings"} onClose={() => setDialog(null)} />
      <ImportDialog open={dialog?.kind === "import"} onClose={() => setDialog(null)} workspace={workspace} onLogin={(integ) => setDialog({ kind: "login", integration: integ })} />
      <RenameDialog
        target={dialog?.kind === "rename" ? { kind: "integration", id: dialog.integration.id, name: dialog.integration.alias, save: (n) => api.RenameIntegration(dialog.integration.id, n) } : null}
        onClose={() => setDialog(null)}
      />
    </div>
  );
}

function SideItem({ active, onClick, label, count, dot, icon, trailing }: { active: boolean; onClick: () => void; label: string; count?: number; dot?: "ok" | "off"; icon?: React.ReactNode; trailing?: React.ReactNode }) {
  return (
    <div className={cn("group flex items-center rounded-md pr-1 transition-colors", active ? "bg-sidebar-accent text-sidebar-accent-foreground" : "text-muted-foreground hover:bg-sidebar-accent/60 hover:text-foreground")}>
      <button type="button" onClick={onClick} className="flex min-w-0 flex-1 items-center gap-2 px-2 py-1.5 text-left text-sm">
        {icon}
        {dot && <span className={cn("size-1.5 shrink-0 rounded-full", dot === "ok" ? "bg-emerald-400" : "bg-muted-foreground/40")} />}
        <span className="flex-1 truncate">{label}</span>
        {count !== undefined && <span className="text-xs tabular-nums text-muted-foreground/70">{count}</span>}
      </button>
      {/* Fixed slot keeps counts aligned whether or not a row has a control. */}
      <span className="flex size-6 shrink-0 items-center justify-center">{trailing}</span>
    </div>
  );
}

function Empty({ hasAny, onImport, onConnect }: { hasAny: boolean; onImport: () => void; onConnect: () => void }) {
  return (
    <div className="flex h-full flex-col items-center justify-center text-center">
      <div className="rounded-2xl bg-card p-4"><Mark className="size-10" /></div>
      <h3 className="mt-5 text-lg font-medium">{hasAny ? "Nothing matches" : "No sessions yet"}</h3>
      <p className="mt-1 max-w-sm text-sm text-muted-foreground">{hasAny ? "Try a different search or filter." : "Bring in what your cloud CLIs already know, or connect a cloud from the Add menu."}</p>
      {!hasAny && (
        <div className="mt-5 flex gap-2">
          <Button className="gap-1.5" onClick={onImport}><Import className="size-4" /> Import from this machine</Button>
          <Button variant="secondary" className="gap-1.5" onClick={onConnect}><ShieldCheck className="size-4" /> Connect AWS Identity Center</Button>
        </div>
      )}
    </div>
  );
}
