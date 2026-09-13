import { useState } from "react";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { api, errorMessage } from "@/lib/api";

/** What to remove: the sections of the credentials file, and the file's path for the text. */
export type RemoveKeysTarget = { path: string; profiles: string[] };

/** The class every button that opens this dialog shares: the app's destructive outline. */
export const REMOVE_KEYS_BUTTON = "border-destructive/40 text-destructive hover:bg-destructive/10";

/**
 * The one confirmation for deleting static keys from ~/.aws/credentials. It
 * names the sections and the file, says the change cannot be undone, and runs
 * the removal on the red button.
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
  const names = target?.profiles ?? [];
  async function remove() {
    if (!target) return;
    setBusy(true);
    try {
      for (const n of target.profiles) await api.RemoveStaticProfile(n);
      toast.success(`Static keys removed from ${target.path}`);
      onClose();
      onDone?.();
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog open={!!target} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>Remove static keys?</DialogTitle>
          <DialogDescription>Cannot be undone.</DialogDescription>
        </DialogHeader>
        <div className="rounded-md border bg-muted/40 px-3 py-2 text-sm">
          <ul className="max-h-32 space-y-0.5 overflow-y-auto font-mono text-xs">
            {names.map((n) => (
              <li key={n}>[{n}]</li>
            ))}
          </ul>
          <p className="mt-1.5 text-xs text-muted-foreground">{target?.path}</p>
        </div>
        <div className="flex justify-end gap-2">
          <Button variant="outline" onClick={onClose} disabled={busy}>
            Cancel
          </Button>
          <Button variant="destructive" onClick={() => void remove()} disabled={busy}>
            {busy ? <Loader2 className="size-4 animate-spin" /> : "Remove"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
