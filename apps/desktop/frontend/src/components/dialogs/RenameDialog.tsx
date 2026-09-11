import { useEffect, useState } from "react";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { errorMessage } from "@/lib/api";

/** Rename an integration or a session. */
export function RenameDialog({ target, onClose }: { target: { kind: "integration" | "session"; id: string; name: string; save: (name: string) => Promise<unknown> } | null; onClose: () => void }) {
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => { setName(target?.name ?? ""); }, [target]);
  const valid = name.trim().length > 0 && name.trim() !== target?.name;
  return (
    <Dialog open={!!target} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>Rename {target?.kind === "integration" ? "account" : "session"}</DialogTitle>
          <DialogDescription>{target?.kind === "session" ? "An active AWS session moves its profile to the new name." : "Only the display name changes."}</DialogDescription>
        </DialogHeader>
        <form
          className="space-y-3"
          onSubmit={async (e) => {
            e.preventDefault();
            if (!target || !valid) return;
            setBusy(true);
            try {
              await target.save(name.trim());
              toast.success(`Renamed to ${name.trim()}`);
              onClose();
            } catch (err) {
              toast.error(errorMessage(err));
            } finally {
              setBusy(false);
            }
          }}
        >
          <Input value={name} onChange={(e) => setName(e.target.value)} autoFocus onFocus={(e) => e.target.select()} />
          <Button type="submit" className="w-full" disabled={!valid || busy}>{busy ? <Loader2 className="size-4 animate-spin" /> : "Save"}</Button>
        </form>
      </DialogContent>
    </Dialog>
  );
}
