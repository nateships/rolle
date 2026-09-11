import { useEffect, useState } from "react";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { errorMessage } from "@/lib/api";

export type RenameTarget = {
  kind: "integration" | "session" | "profile";
  id: string;
  name: string;
  save: (name: string) => Promise<unknown>;
};

const COPY: Record<RenameTarget["kind"], { title: string; description: string; placeholder?: string; allowEmpty?: boolean; done: (v: string) => string }> = {
  integration: { title: "Rename account", description: "Only the display name changes.", done: (v) => `Renamed to ${v}` },
  session: { title: "Rename session", description: "The AWS profile name does not change.", done: (v) => `Renamed to ${v}` },
  profile: {
    title: "AWS profile name",
    description: "Sessions share the default profile unless you set one here. Letters, digits, '.', '-', and '_'. Leave empty to use default.",
    placeholder: "default",
    allowEmpty: true,
    done: (v) => (v ? `Profile set to ${v}` : "Profile set to default"),
  },
};

/** Rename an integration or session, or set a session's AWS profile name. */
export function RenameDialog({ target, onClose }: { target: RenameTarget | null; onClose: () => void }) {
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const targetId = target?.id;
  const targetName = target?.name;
  useEffect(() => { setName(targetName ?? ""); }, [targetId, targetName]);
  const copy = COPY[target?.kind ?? "session"];
  const valid = (copy.allowEmpty || name.trim().length > 0) && name.trim() !== (target?.name ?? "");
  return (
    <Dialog open={!!target} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>{copy.title}</DialogTitle>
          <DialogDescription>{copy.description}</DialogDescription>
        </DialogHeader>
        <form
          className="space-y-3"
          onSubmit={async (e) => {
            e.preventDefault();
            if (!target || !valid) return;
            setBusy(true);
            try {
              await target.save(name.trim());
              toast.success(copy.done(name.trim()));
              onClose();
            } catch (err) {
              toast.error(errorMessage(err));
            } finally {
              setBusy(false);
            }
          }}
        >
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder={copy.placeholder} autoFocus onFocus={(e) => e.target.select()} className={target?.kind === "profile" ? "font-mono" : undefined} />
          <Button type="submit" className="w-full" disabled={!valid || busy}>{busy ? <Loader2 className="size-4 animate-spin" /> : "Save"}</Button>
        </form>
      </DialogContent>
    </Dialog>
  );
}
