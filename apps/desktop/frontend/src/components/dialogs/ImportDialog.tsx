import { useState } from "react";
import { Loader2, RefreshCw } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { FoundList, tenantAlias, useDiscovery } from "@/components/discovery/FoundList";
import { api, errorMessage, type Integration, type Workspace } from "@/lib/api";
import { celebrate } from "@/lib/celebrate";

/** Import identities other CLIs already hold, from the dashboard. */
export function ImportDialog({ open, onClose, workspace, onLogin }: { open: boolean; onClose: () => void; workspace: Workspace; onLogin: (i: Integration) => void }) {
  const d = useDiscovery(workspace, open);
  const [importing, setImporting] = useState<string | null>(null);

  const guard = async (key: string, fn: () => Promise<void>) => {
    setImporting(key);
    try {
      await fn();
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setImporting(null);
    }
  };

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Import from this machine</DialogTitle>
          <DialogDescription>Identities the AWS, Azure, and Google CLIs already know about.</DialogDescription>
        </DialogHeader>
        {d.loading ? (
          <div className="flex items-center gap-2 py-6 text-sm text-muted-foreground"><Loader2 className="size-4 animate-spin" /> Scanning…</div>
        ) : d.count === 0 ? (
          <div className="space-y-3 py-4 text-center text-sm text-muted-foreground">
            <p>Nothing new found. Everything the CLIs know is already in Rolle, or no CLI is signed in.</p>
            <Button variant="secondary" size="sm" className="gap-1.5" onClick={d.rescan}><RefreshCw className="size-3.5" /> Scan again</Button>
          </div>
        ) : (
          <FoundList
            portals={d.portals}
            tenants={d.tenants}
            gcp={d.gcp}
            importing={importing}
            disabled={importing !== null}
            onAWS={(p) => guard(p.startUrl, async () => {
              const res = await api.ImportAWSSSO(p.alias, p.startUrl, p.region);
              if (res.loggedIn) {
                toast.success(`Imported ${p.alias}`, { description: `${res.sessions?.length ?? 0} role${res.sessions?.length === 1 ? "" : "s"} from your AWS CLI sign-in.` });
                if ((res.sessions?.length ?? 0) > 0) celebrate("small");
              } else {
                onClose();
                onLogin(res.integration);
              }
            })}
            onAzure={(t) => guard(t.tenantId, async () => {
              const integ = await api.AddAzure(tenantAlias(t), t.tenantId);
              onClose();
              onLogin(integ);
            })}
            onGCP={() => guard("gcp", async () => {
              const added = (await api.AddGCP("gcp")) ?? [];
              toast.success(`${added.length} project${added.length === 1 ? "" : "s"} discovered`);
              if (added.length > 0) celebrate("small");
            })}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
