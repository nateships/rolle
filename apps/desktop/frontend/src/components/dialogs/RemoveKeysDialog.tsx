import { useEffect, useState } from "react";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { api, errorMessage } from "@/lib/api";

/** The profiles to remove from the credentials file. */
export type RemoveKeysTarget = { profiles: string[] };

/**
 * The class every button that opens this dialog shares: the app's destructive
 * outline, which stays red on hover.
 */
export const REMOVE_KEYS_BUTTON =
  "border-destructive/40 text-destructive hover:border-destructive/60 hover:bg-destructive/10 hover:text-destructive";

type Listing = Awaited<ReturnType<typeof api.StaticProfiles>>;

/**
 * The one confirmation for removing profiles with static keys from
 * ~/.aws/credentials. It shows the file, then each profile as it sits in the
 * file with the values masked, and runs the removal on the red button.
 */
export function RemoveKeysDialog({
  target,
  onClose,
  onDone,
}: {
  target: RemoveKeysTarget | null;
  onClose: () => void;
  onDone?: () => void;
}) {
  const [busy, setBusy] = useState(false);
  const [listing, setListing] = useState<Listing | null>(null);
  const open = !!target;
  useEffect(() => {
    if (!open) return;
    void api
      .StaticProfiles()
      .then(setListing)
      .catch((e) => toast.error(errorMessage(e)));
  }, [open]);
  const shown = (listing?.profiles ?? []).filter((p) => target?.profiles.includes(p.name));

  async function remove() {
    if (!target) return;
    setBusy(true);
    try {
      for (const n of target.profiles) await api.RemoveStaticProfile(n);
      toast.success(`Removed from ${listing?.path ?? "~/.aws/credentials"}`);
      onClose();
      onDone?.();
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>
            Remove {target?.profiles.length === 1 ? "profile" : "profiles"} from the credentials file?
          </DialogTitle>
          <DialogDescription>Cannot be undone.</DialogDescription>
        </DialogHeader>
        {listing && (
          <div className="space-y-1.5">
            <p className="font-mono text-xs text-muted-foreground">{listing.path}</p>
            <pre className="max-h-48 overflow-y-auto rounded-md border bg-muted/40 px-3 py-2 font-mono text-xs leading-5">
              {shown.map((p, i) => (
                <div key={p.name} className={i > 0 ? "mt-2" : undefined}>
                  <div>[{p.name}]</div>
                  {(p.keys ?? []).map((k) => (
                    <div key={k.name} className="text-muted-foreground">
                      {k.name} = {k.preview}
                    </div>
                  ))}
                </div>
              ))}
            </pre>
          </div>
        )}
        <div className="flex justify-end gap-2">
          <Button variant="outline" onClick={onClose} disabled={busy}>
            Cancel
          </Button>
          <Button variant="destructive" onClick={() => void remove()} disabled={busy || !listing}>
            {busy ? <Loader2 className="size-4 animate-spin" /> : "Remove"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
