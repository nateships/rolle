import type { ComponentType } from "react";
import {
  Briefcase,
  Building2,
  Cloud,
  FlaskConical,
  Folder,
  Rocket,
  Shield,
  Star,
  Tag as TagIcon,
  Wrench,
} from "lucide-react";
import type { Tag } from "@/lib/api";
import { cn } from "@/lib/utils";

/** The palette a tag may use. Keys match core.TagColors on the backend. */
export const TAG_COLORS: Record<string, { text: string; swatch: string }> = {
  gray: { text: "text-muted-foreground", swatch: "bg-muted-foreground" },
  blue: { text: "text-brand-blue", swatch: "bg-brand-blue" },
  green: { text: "text-brand-green", swatch: "bg-brand-green" },
  orange: { text: "text-brand-orange", swatch: "bg-brand-orange" },
  red: { text: "text-red-500", swatch: "bg-red-500" },
  purple: { text: "text-purple-500", swatch: "bg-purple-500" },
  pink: { text: "text-pink-500", swatch: "bg-pink-500" },
  yellow: { text: "text-yellow-500", swatch: "bg-yellow-500" },
};

/** The icons a tag may use. Keys match core.TagIcons on the backend. */
export const TAG_ICONS: Record<string, ComponentType<{ className?: string }>> = {
  tag: TagIcon,
  folder: Folder,
  briefcase: Briefcase,
  shield: Shield,
  flask: FlaskConical,
  rocket: Rocket,
  star: Star,
  building: Building2,
  cloud: Cloud,
  wrench: Wrench,
};

export const tagColorClass = (color?: string | null) => (TAG_COLORS[color ?? ""] ?? TAG_COLORS.gray).text;

/** The tag's icon in its color. */
export function TagGlyph({ tag, className }: { tag: Pick<Tag, "color" | "icon">; className?: string }) {
  const Icon = TAG_ICONS[tag.icon ?? ""] ?? TagIcon;
  return <Icon className={cn(tagColorClass(tag.color), className)} aria-hidden />;
}
