import { useEffect, useState } from "react";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { api, errorMessage } from "@/lib/api";

type Listing = Awaited<ReturnType<typeof api.ForeignProfile>>;

/**
 * The confirmation for a profile of ~/.aws/config that another tool
 * configures. It shows the lines of the other tool as they sit in the file,
 * and removes them on the red button. rolle can then write the profile.
 */
export function TakeOverProfileDialog({
  profile,
  onClose,
  onDone,
}: {
  /** The profile to take over. Null closes the dialog. */
  profile: string | null;
  onClose: () => void;
  onDone?: () => void;
}) {
  const [busy, setBusy] = useState(false);
  const [listing, setListing] = useState<Listing | null>(null);
  useEffect(() => {
    if (!profile) return;
    void api
      .ForeignProfile(profile)
      .then(setListing)
      .catch((e) => toast.error(errorMessage(e)));
  }, [profile]);
  const keys = listing?.keys ?? [];

  async function takeOver() {
    if (!profile || !listing) return;
    setBusy(true);
    try {
      await api.RemoveForeignKeys(profile);
      toast.success(`Removed from ${listing.path}`);
      // The lines are gone. The next open reads the file again.
      setListing(null);
      onClose();
      onDone?.();
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog open={!!profile} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>Take over profile {profile}?</DialogTitle>
          <DialogDescription>
            rolle removes the settings of the other tool from the config file. Cannot be undone.
          </DialogDescription>
        </DialogHeader>
        {listing && keys.length === 0 && (
          <p className="text-sm text-muted-foreground">
            No other tool configures {profile} in <span className="font-mono text-xs">{listing.path}</span>.
          </p>
        )}
        {listing && keys.length > 0 && (
          <div className="space-y-1.5">
            <p className="font-mono text-xs text-muted-foreground">{listing.path}</p>
            <pre className="max-h-48 overflow-y-auto rounded-md border bg-muted/40 px-3 py-2 font-mono text-xs leading-5">
              <div>[{profile === "default" ? "default" : `profile ${profile}`}]</div>
              {keys.map((k) => (
                <div key={k.name} className="text-muted-foreground">
                  {k.name} = {k.value}
                </div>
              ))}
            </pre>
          </div>
        )}
        <div className="flex justify-end gap-2">
          <Button variant="outline" onClick={onClose} disabled={busy}>
            Cancel
          </Button>
          <Button variant="destructive" onClick={() => void takeOver()} disabled={busy || keys.length === 0}>
            {busy ? <Loader2 className="size-4 animate-spin" /> : "Take over"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
