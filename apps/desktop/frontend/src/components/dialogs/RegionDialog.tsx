import { useEffect, useState } from "react";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { RegionSelect } from "@/components/RegionSelect";
import { api, errorMessage, type Session } from "@/lib/api";

/** Change the region of an AWS session. */
export function RegionDialog({ session, onClose }: { session: Session | null; onClose: () => void }) {
  const [region, setRegion] = useState("");
  const [busy, setBusy] = useState(false);
  const current = session?.region ?? "";
  // The id is a deliberate extra dependency: a new session with the same region still resets the picker.
  /* oxlint-disable react/exhaustive-effect-dependencies */
  useEffect(() => {
    setRegion(current);
  }, [session?.id, current]);
  /* oxlint-enable react/exhaustive-effect-dependencies */
  const valid = region !== "" && region !== current;
  return (
    <Dialog open={!!session} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>Change region</DialogTitle>
          <DialogDescription>An active session rewrites its profile with the new region.</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <RegionSelect value={region} onChange={setRegion} />
          <Button
            className="w-full"
            disabled={!valid || busy}
            onClick={async () => {
              if (!session) return;
              setBusy(true);
              try {
                await api.SetRegion(session.id, region);
                toast.success(`Region set to ${region}`);
                onClose();
              } catch (err) {
                toast.error(errorMessage(err));
              } finally {
                setBusy(false);
              }
            }}
          >
            {busy ? <Loader2 className="size-4 animate-spin" /> : "Save"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
