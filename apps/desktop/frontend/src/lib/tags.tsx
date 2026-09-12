import type { ReactElement } from "react";
import { Tag as TagIcon } from "lucide-react";
import { DynamicIcon, iconNames, type IconName } from "lucide-react/dynamic";
import type { Tag } from "@/lib/api";
import { cn } from "@/lib/utils";

/** Quick picks in the tag dialog; the color input takes any other value. */
export const TAG_PRESETS: { name: string; hex: string }[] = [
  { name: "gray", hex: "#8b9099" },
  { name: "blue", hex: "#244cff" },
  { name: "green", hex: "#00ce78" },
  { name: "teal", hex: "#12a5b8" },
  { name: "orange", hex: "#ff7900" },
  { name: "red", hex: "#e5484d" },
  { name: "purple", hex: "#8e4ec6" },
  { name: "pink", hex: "#e93d82" },
  { name: "yellow", hex: "#f5b400" },
];

export const DEFAULT_TAG_COLOR = TAG_PRESETS[0].hex;

const names = new Set<string>(iconNames);

/** Every Lucide icon name, for the picker's search. */
export const ICON_NAMES: string[] = iconNames;

export const isIconName = (name: string): name is IconName => names.has(name);

// DynamicIcon loads the icon in an effect and renders a fallback until then. The
// fallback holds the icon's space; one component per class list keeps it stable.
const blanks = new Map<string, () => ReactElement>();
function blank(cls: string) {
  let b = blanks.get(cls);
  if (!b) {
    b = () => <span className={cn("inline-block", cls)} aria-hidden />;
    blanks.set(cls, b);
  }
  return b;
}

/** The tag's icon in its color. An unknown or empty icon falls back to the tag glyph. */
export function TagGlyph({ tag, className }: { tag: Pick<Tag, "color" | "icon">; className?: string }) {
  const style = { color: tag.color || DEFAULT_TAG_COLOR };
  const cls = cn("shrink-0", className);
  if (!tag.icon || !isIconName(tag.icon)) return <TagIcon className={cls} style={style} aria-hidden />;
  return <DynamicIcon name={tag.icon} className={cls} style={style} aria-hidden fallback={blank(cls)} />;
}
