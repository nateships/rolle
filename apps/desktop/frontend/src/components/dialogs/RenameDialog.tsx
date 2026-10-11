import { useEffect, useState } from "react";
import { Copy, Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { REMOVE_KEYS_BUTTON } from "@/components/dialogs/RemoveKeysDialog";
import { errorMessage } from "@/lib/api";
import { copyText } from "@/lib/clipboard";
import { cn } from "@/lib/utils";

export type RenameTarget = {
  kind: "integration" | "session" | "profile";
  id: string;
  name: string;
  save: (name: string) => Promise<unknown>;
  /**
   * A second way to save: the name becomes an alias for the permission set
   * in every account. The toggle swaps the field to the role part alone.
   */
  everywhere?: { label: string; name: string; save: (name: string) => Promise<unknown> };
  /** Looks at the typed name and returns a warning to show, or "". */
  check?: (name: string) => Promise<string>;
  /** Opens the fix for what the warning is about, with the typed name. */
  onFix?: (name: string) => void;
  /** Replaces the kind's description. */
  description?: string;
  /**
   * Fields below the name, such as a portal's start URL. Each has a copy
   * button. A field with save is editable, and its note shows once it changes.
   */
  details?: { label: string; value: string; save?: (value: string) => Promise<unknown>; note?: string }[];
};

const COPY: Record<
  RenameTarget["kind"],
  { title: string; description: string; placeholder?: string; allowEmpty?: boolean; done: (v: string) => string }
> = {
  integration: {
    title: "Edit account",
    description: "Only the display name changes.",
    done: (v) => `Renamed to ${v}`,
  },
  session: {
    title: "Rename session",
    description: "The AWS profile name does not change.",
    done: (v) => `Renamed to ${v}`,
  },
  profile: {
    title: "AWS profile name",
    description:
      "Sessions share the default profile unless you set one here. Letters, digits, '.', '-', and '_'. Leave empty to use default.",
    placeholder: "default",
    allowEmpty: true,
    done: (v) => (v ? `Profile set to ${v}` : "Profile set to default"),
  },
};

/** Rename an integration or session, or set a session's AWS profile name. */
export function RenameDialog({ target, onClose }: { target: RenameTarget | null; onClose: () => void }) {
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [all, setAll] = useState(false);
  const [warning, setWarning] = useState("");
  // The typed value of each detail, by index. A detail without one shows its stored value.
  const [edits, setEdits] = useState<Record<number, string>>({});
  const check = target?.check;
  // The check runs a moment after typing stops.
  useEffect(() => {
    if (!check) return;
    const t = setTimeout(() => void check(name.trim()).then(setWarning, () => setWarning("")), 250);
    return () => clearTimeout(t);
  }, [check, name]);
  const targetId = target?.id;
  const targetName = target?.name;
  // The id is a deliberate extra dependency: a new target with the same name still resets the field.
  /* oxlint-disable react/exhaustive-effect-dependencies */
  useEffect(() => {
    setName(targetName ?? "");
    setAll(false);
    setWarning("");
    setEdits({});
  }, [targetId, targetName]);
  /* oxlint-enable react/exhaustive-effect-dependencies */
  const copy = COPY[target?.kind ?? "session"];
  const everywhere = all ? target?.everywhere : undefined;
  const initial = everywhere?.name ?? target?.name ?? "";
  const renamed = name.trim() !== initial;
  // The details that the user changed, with the new value.
  const changed = (target?.details ?? []).flatMap((d, i) => {
    const v = (edits[i] ?? d.value).trim();
    return d.save && v !== d.value ? [{ save: d.save, value: v }] : [];
  });
  const valid =
    (copy.allowEmpty || name.trim().length > 0) && (renamed || changed.length > 0) && changed.every((c) => c.value);
  const toggleAll = (on: boolean) => {
    setAll(on);
    setName(on ? (target?.everywhere?.name ?? "") : (target?.name ?? ""));
  };
  return (
    <Dialog open={!!target} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>{copy.title}</DialogTitle>
          <DialogDescription>{target?.description ?? copy.description}</DialogDescription>
        </DialogHeader>
        <form
          className="space-y-3"
          onSubmit={async (e) => {
            e.preventDefault();
            if (!target || !valid) return;
            setBusy(true);
            try {
              // The details save first: the backend checks them, and a refusal then leaves the name as it was.
              for (const c of changed) await c.save(c.value);
              if (renamed) await (everywhere ? everywhere.save(name.trim()) : target.save(name.trim()));
              toast.success(
                !renamed
                  ? "Saved"
                  : everywhere
                    ? `${everywhere.name} is now ${name.trim()} in every account`
                    : copy.done(name.trim()),
              );
              onClose();
            } catch (err) {
              toast.error(errorMessage(err));
            } finally {
              setBusy(false);
            }
          }}
        >
          {target?.details && (
            <Label htmlFor="rename-name" className="text-xs text-muted-foreground">
              Name
            </Label>
          )}
          <Input
            id="rename-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={copy.placeholder}
            autoFocus
            onFocus={(e) => e.target.select()}
            className={target?.kind === "profile" ? "font-mono" : undefined}
          />
          {warning && (
            <div
              role="alert"
              className="flex items-center justify-between gap-3 text-xs text-amber-600 dark:text-amber-400"
            >
              <span>{warning}</span>
              {target?.onFix && (
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  className={cn("h-7 shrink-0", REMOVE_KEYS_BUTTON)}
                  onClick={() => target.onFix?.(name.trim())}
                >
                  Remove
                </Button>
              )}
            </div>
          )}
          {target?.details?.map((d, i) => (
            <div key={d.label} className="space-y-1.5">
              <Label htmlFor={`rename-detail-${i}`} className="text-xs text-muted-foreground">
                {d.label}
              </Label>
              <div className="flex gap-2">
                <Input
                  id={`rename-detail-${i}`}
                  value={edits[i] ?? d.value}
                  onChange={(e) => setEdits((prev) => ({ ...prev, [i]: e.target.value }))}
                  readOnly={!d.save}
                  disabled={!d.save}
                  className="font-mono text-xs"
                />
                <Button
                  type="button"
                  variant="outline"
                  size="icon"
                  className="shrink-0"
                  aria-label={`Copy ${d.label}`}
                  title={`Copy ${d.label}`}
                  onClick={() =>
                    copyText((edits[i] ?? d.value).trim()).then(
                      () => toast.success("Copied", { description: (edits[i] ?? d.value).trim() }),
                      (err) => toast.error(errorMessage(err)),
                    )
                  }
                >
                  <Copy />
                </Button>
              </div>
              {d.note && (edits[i] ?? d.value).trim() !== d.value && (
                <p role="note" className="text-xs text-amber-600 dark:text-amber-400">
                  {d.note}
                </p>
              )}
            </div>
          ))}
          {target?.everywhere && (
            <div className="flex items-center justify-between gap-3 text-sm">
              <Label htmlFor="rename-everywhere" className="font-normal text-muted-foreground">
                {target.everywhere.label}
              </Label>
              <Switch id="rename-everywhere" checked={all} onCheckedChange={toggleAll} />
            </div>
          )}
          <Button type="submit" className="w-full" disabled={!valid || busy}>
            {busy ? <Loader2 className="size-4 animate-spin" /> : "Save"}
          </Button>
        </form>
      </DialogContent>
    </Dialog>
  );
}
