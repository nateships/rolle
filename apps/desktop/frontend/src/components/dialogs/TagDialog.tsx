import { useMemo, useState } from "react";
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
import { DEFAULT_TAG_COLOR, ICON_NAMES, TAG_PRESETS, TagGlyph } from "@/lib/tags";
import { cn } from "@/lib/utils";

export type TagTarget = { kind: "new" } | { kind: "edit"; tag: Tag };

/** Create a tag or change its name, color, and icon. onRenamed reports a name change. */
export function TagDialog({
  target,
  onClose,
  onRenamed,
}: {
  target: TagTarget | null;
  onClose: () => void;
  onRenamed?: (from: string, to: string) => void;
}) {
  const editing = target?.kind === "edit" ? target.tag : null;
  return (
    <Dialog open={target !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{editing ? "Edit tag" : "New tag"}</DialogTitle>
          <DialogDescription>
            A tag groups sessions in the sidebar. Drag sessions onto it, or tag them from their menu.
          </DialogDescription>
        </DialogHeader>
        {/* The key resets the form for each target the dialog opens with. */}
        {target && <TagForm key={editing?.name ?? "new"} editing={editing} onClose={onClose} onRenamed={onRenamed} />}
      </DialogContent>
    </Dialog>
  );
}

/** The grid renders this many icons at a time and adds a batch when it scrolls near the end. */
const ICON_BATCH = 96;

function TagForm({
  editing,
  onClose,
  onRenamed,
}: {
  editing: Tag | null;
  onClose: () => void;
  onRenamed?: (from: string, to: string) => void;
}) {
  const [name, setName] = useState(editing?.name ?? "");
  const [color, setColor] = useState(editing?.color || DEFAULT_TAG_COLOR);
  const [icon, setIcon] = useState(editing?.icon || "tag");
  const [pickerOpen, setPickerOpen] = useState(false);
  const [busy, setBusy] = useState(false);

  async function save() {
    setBusy(true);
    try {
      const tag: Tag = { name: name.trim(), color, icon };
      if (editing) await api.UpdateTag(editing.name, tag);
      else await api.AddTag(tag);
      if (editing && editing.name !== tag.name) onRenamed?.(editing.name, tag.name);
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
        <div className="flex items-center gap-2">
          <Button
            type="button"
            variant="outline"
            size="icon"
            aria-label="Choose icon and color"
            aria-expanded={pickerOpen}
            onClick={() => setPickerOpen((o) => !o)}
          >
            <TagGlyph tag={{ color, icon }} className="size-4" />
          </Button>
          <Input
            id="tag-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Production"
            autoFocus
            className="flex-1"
          />
        </div>
        {pickerOpen && <IconPanel color={color} icon={icon} onColor={setColor} onIcon={setIcon} />}
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

/** A search over every Lucide icon and the color choices. It sits inline in
 * the dialog rather than in a popover: the dialog's scroll lock would swallow
 * wheel events in a portal. */
function IconPanel({
  color,
  icon,
  onColor,
  onIcon,
}: {
  color: string;
  icon: string;
  onColor: (c: string) => void;
  onIcon: (i: string) => void;
}) {
  const [query, setQuery] = useState("");
  const matches = useMemo(() => {
    const q = query.trim().toLowerCase();
    return q ? ICON_NAMES.filter((n) => n.includes(q)) : ICON_NAMES;
  }, [query]);
  const [shown, setShown] = useState(ICON_BATCH);
  const results = matches.slice(0, shown);
  const custom = !TAG_PRESETS.some((p) => p.hex === color);
  return (
    <div className="min-w-0 space-y-3 rounded-md border bg-muted/30 p-3">
      <Input
        value={query}
        onChange={(e) => {
          setQuery(e.target.value);
          setShown(ICON_BATCH);
        }}
        placeholder="Search icons"
        aria-label="Search icons"
        autoFocus
      />
      <div
        className="grid max-h-48 grid-cols-8 gap-1 overflow-y-auto pr-1"
        role="listbox"
        aria-label="Icons"
        onScroll={(e) => {
          const el = e.currentTarget;
          if (el.scrollTop + el.clientHeight >= el.scrollHeight - 64) setShown((n) => n + ICON_BATCH);
        }}
      >
        {results.map((n) => (
          <button
            key={n}
            type="button"
            role="option"
            aria-selected={icon === n}
            aria-label={n}
            title={n}
            onClick={() => onIcon(n)}
            className={cn(
              "flex size-8 items-center justify-center rounded-md transition-colors hover:bg-muted",
              icon === n && "bg-muted ring-1 ring-ring",
            )}
          >
            <TagGlyph tag={{ color, icon: n }} className="size-4" />
          </button>
        ))}
        {results.length === 0 && (
          <p className="col-span-8 py-2 text-center text-xs text-muted-foreground">No icon matches</p>
        )}
      </div>
      <div className="flex min-w-0 flex-wrap items-center gap-2">
        {TAG_PRESETS.map((p) => (
          <button
            key={p.hex}
            type="button"
            aria-label={p.name}
            aria-pressed={color === p.hex}
            onClick={() => onColor(p.hex)}
            style={{ backgroundColor: p.hex }}
            className={cn(
              "size-5 rounded-full ring-offset-2 ring-offset-background transition-shadow",
              color === p.hex && "ring-2 ring-ring",
            )}
          />
        ))}
        {/* The color input is the rainbow swatch, so a click opens the native picker directly. */}
        <input
          type="color"
          value={color}
          onChange={(e) => onColor(e.target.value)}
          aria-label="Custom color"
          aria-pressed={custom}
          title="Custom color"
          style={{ background: "conic-gradient(#f00, #ff0, #0f0, #0ff, #00f, #f0f, #f00)" }}
          className={cn(
            "ml-auto size-5 cursor-pointer appearance-none rounded-full border-0 p-0 ring-offset-2 ring-offset-background",
            "[&::-webkit-color-swatch-wrapper]:p-0 [&::-webkit-color-swatch]:rounded-full [&::-webkit-color-swatch]:border-0 [&::-webkit-color-swatch]:opacity-0",
            custom && "ring-2 ring-ring",
          )}
        />
        <Input
          value={color}
          onChange={(e) => onColor(e.target.value)}
          aria-label="Color value"
          className="h-7 w-24 font-mono text-xs"
          spellCheck={false}
        />
      </div>
    </div>
  );
}
