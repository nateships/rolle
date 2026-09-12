import { useState } from "react";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api, errorMessage, type Tag } from "@/lib/api";
import { TAG_COLORS, TAG_ICONS, TagGlyph } from "@/lib/tags";
import { cn } from "@/lib/utils";

export type TagTarget = { kind: "new" } | { kind: "edit"; tag: Tag };

/** Create a tag or change its name, color, and icon. */
export function TagDialog({ target, onClose }: { target: TagTarget | null; onClose: () => void }) {
  const editing = target?.kind === "edit" ? target.tag : null;
  return (
    <Dialog open={target !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>{editing ? "Edit tag" : "New tag"}</DialogTitle>
          <DialogDescription>
            A tag groups sessions in the sidebar. Drag sessions onto it, or tag them from their menu.
          </DialogDescription>
        </DialogHeader>
        {/* The key resets the form for each target the dialog opens with. */}
        {target && <TagForm key={editing?.name ?? "new"} editing={editing} onClose={onClose} />}
      </DialogContent>
    </Dialog>
  );
}

function TagForm({ editing, onClose }: { editing: Tag | null; onClose: () => void }) {
  const [name, setName] = useState(editing?.name ?? "");
  const [color, setColor] = useState(editing?.color || "gray");
  const [icon, setIcon] = useState(editing?.icon || "tag");
  const [busy, setBusy] = useState(false);

  async function save() {
    setBusy(true);
    try {
      const tag = { name: name.trim(), color, icon } as Tag;
      if (editing) await api.UpdateTag(editing.name, tag);
      else await api.AddTag(tag);
      toast.success(editing ? `Tag ${tag.name} updated` : `Tag ${tag.name} created`);
      onClose();
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        if (name.trim()) void save();
      }}
    >
      <div className="space-y-1.5">
        <Label htmlFor="tag-name">Name</Label>
        <Input
          id="tag-name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Production"
          autoFocus
        />
      </div>
      <div className="space-y-1.5">
        <Label>Color</Label>
        <div className="flex flex-wrap gap-2">
          {Object.entries(TAG_COLORS).map(([c, v]) => (
            <button
              key={c}
              type="button"
              aria-label={c}
              aria-pressed={color === c}
              onClick={() => setColor(c)}
              className={cn(
                "size-6 rounded-full ring-offset-2 ring-offset-background transition-shadow",
                v.swatch,
                color === c && "ring-2 ring-ring",
              )}
            />
          ))}
        </div>
      </div>
      <div className="space-y-1.5">
        <Label>Icon</Label>
        <div className="flex flex-wrap gap-1">
          {Object.keys(TAG_ICONS).map((i) => (
            <button
              key={i}
              type="button"
              aria-label={i}
              aria-pressed={icon === i}
              onClick={() => setIcon(i)}
              className={cn(
                "flex size-8 items-center justify-center rounded-md border transition-colors hover:bg-muted",
                icon === i && "border-ring bg-muted",
              )}
            >
              <TagGlyph tag={{ color, icon: i }} className="size-4" />
            </button>
          ))}
        </div>
      </div>
      <DialogFooter>
        <Button type="button" variant="ghost" onClick={onClose}>
          Cancel
        </Button>
        <Button type="submit" disabled={busy || !name.trim()}>
          {busy ? <Loader2 className="size-4 animate-spin" /> : "Save"}
        </Button>
      </DialogFooter>
    </form>
  );
}
