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
  const list = names.length > 3 ? `${names.length} sections` : names.map((n) => `[${n}]`).join(", ");
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
          <DialogTitle>Remove static keys</DialogTitle>
          <DialogDescription>
            Deletes the access key lines of {list} from {target?.path}. Other lines stay. This cannot be undone.
          </DialogDescription>
        </DialogHeader>
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
