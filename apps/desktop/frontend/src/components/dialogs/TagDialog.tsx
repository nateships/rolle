import { useMemo, useRef, useState } from "react";
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
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { api, errorMessage, type Tag } from "@/lib/api";
import { DEFAULT_TAG_COLOR, ICON_NAMES, TAG_PRESETS, TagGlyph } from "@/lib/tags";
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

const MAX_ICON_RESULTS = 160;

function TagForm({ editing, onClose }: { editing: Tag | null; onClose: () => void }) {
  const [name, setName] = useState(editing?.name ?? "");
  const [color, setColor] = useState(editing?.color || DEFAULT_TAG_COLOR);
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
        <div className="flex items-center gap-2">
          <IconPicker color={color} icon={icon} onColor={setColor} onIcon={setIcon} />
          <Input
            id="tag-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Production"
            autoFocus
            className="flex-1"
          />
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

/** The tag's icon as a button; it opens a search over every Lucide icon and the color choices. */
function IconPicker({
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
  const results = useMemo(() => {
    const q = query.trim().toLowerCase();
    const list = q ? ICON_NAMES.filter((n) => n.includes(q)) : ICON_NAMES;
    return list.slice(0, MAX_ICON_RESULTS);
  }, [query]);
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button type="button" variant="outline" size="icon" aria-label="Choose icon and color">
          <TagGlyph tag={{ color, icon }} className="size-4" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-72 space-y-3">
        <Input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search icons"
          aria-label="Search icons"
          autoFocus
        />
        <div className="grid max-h-56 grid-cols-8 gap-1 overflow-y-auto pr-1" role="listbox" aria-label="Icons">
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
        {results.length === MAX_ICON_RESULTS && (
          <p className="text-[11px] text-muted-foreground">Showing the first {MAX_ICON_RESULTS}. Type to narrow.</p>
        )}
        <div className="flex items-center gap-2">
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
          <CustomColor color={color} custom={!TAG_PRESETS.some((p) => p.hex === color)} onColor={onColor} />
        </div>
      </PopoverContent>
    </Popover>
  );
}

/** A rainbow ring, like the system swatch, that opens the native color picker. */
function CustomColor({ color, custom, onColor }: { color: string; custom: boolean; onColor: (c: string) => void }) {
  const input = useRef<HTMLInputElement>(null);
  return (
    <span className="relative ml-auto">
      <button
        type="button"
        aria-label="Custom color"
        aria-pressed={custom}
        onClick={() => input.current?.click()}
        style={{ background: "conic-gradient(#f00, #ff0, #0f0, #0ff, #00f, #f0f, #f00)" }}
        className={cn(
          "flex size-5 items-center justify-center rounded-full ring-offset-2 ring-offset-background transition-shadow",
          custom && "ring-2 ring-ring",
        )}
      >
        {custom && <span className="size-2.5 rounded-full border border-white/70" style={{ backgroundColor: color }} />}
      </button>
      <input
        ref={input}
        type="color"
        value={color}
        onChange={(e) => onColor(e.target.value)}
        tabIndex={-1}
        aria-hidden
        className="absolute inset-0 size-0 opacity-0"
      />
    </span>
  );
}
