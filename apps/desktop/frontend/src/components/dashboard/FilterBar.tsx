import { useMemo } from "react";
import { Bookmark, Cloud, Globe, Server, Tag as TagIcon, Timer, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Status, type Integration, type Session, type Tag } from "@/lib/api";
import { cloudOf, kindLabel } from "@/lib/format";
import { cn } from "@/lib/utils";

/** The facets a chip can narrow the table by. Each holds the picked values; empty means any. */
export type Refine = {
  clouds: string[];
  integrations: string[];
  regions: string[];
  tags: string[];
  kinds: string[];
  states: string[];
};
export type Facet = keyof Refine;

export const EMPTY_REFINE: Refine = { clouds: [], integrations: [], regions: [], tags: [], kinds: [], states: [] };

/** How many values are picked across every facet. */
export const refineCount = (r: Refine) => Object.values(r).reduce((n, v) => n + v.length, 0);

const CLOUD_LABELS = { aws: "AWS", azure: "Azure", gcp: "Google Cloud" } as const;
const STATE_LABELS = { active: "Active", expired: "Expired", inactive: "Inactive" } as const;

/** The state a session shows in the State column. */
function stateOf(s: Session): keyof typeof STATE_LABELS {
  if (s.status === Status.StatusActive) return "active";
  return s.expiredAt ? "expired" : "inactive";
}

/** Whether a session passes every picked value. A facet with nothing picked lets every session through. */
export function matchesRefine(s: Session, r: Refine): boolean {
  const any = (picked: string[], value: string | null | undefined) =>
    picked.length === 0 || picked.includes(value ?? "");
  return (
    any(r.clouds, cloudOf(s.kind)) &&
    any(r.integrations, s.integrationId) &&
    any(r.regions, s.region) &&
    any(r.kinds, s.kind) &&
    any(r.states, stateOf(s)) &&
    (r.tags.length === 0 || r.tags.some((t) => (s.tags ?? []).includes(t)))
  );
}

type Option = { value: string; label: string };

/** A row of chips under the header. Each chip opens a list of values to pick from. */
export function FilterBar({
  sessions,
  integrations,
  tags,
  value,
  onChange,
}: {
  /** The sessions the chips draw their values from: the ones that can show at all. */
  sessions: Session[];
  integrations: Integration[];
  tags: Tag[];
  value: Refine;
  onChange: (r: Refine) => void;
}) {
  // The values that exist among the sessions. A facet with one value cannot narrow anything and stays out.
  const facets = useMemo(() => {
    const distinct = (pick: (s: Session) => string | null | undefined) =>
      [...new Set(sessions.map(pick).filter((v): v is string => !!v))].sort();
    const clouds = distinct((s) => cloudOf(s.kind)).map((v) => ({
      value: v,
      label: CLOUD_LABELS[v as keyof typeof CLOUD_LABELS],
    }));
    const kinds = distinct((s) => s.kind).map((v) => ({ value: v, label: kindLabel[v] ?? v }));
    const states = (Object.keys(STATE_LABELS) as (keyof typeof STATE_LABELS)[])
      .filter((k) => sessions.some((s) => stateOf(s) === k))
      .map((v) => ({ value: v, label: STATE_LABELS[v] }));
    const regions = distinct((s) => s.region).map((v) => ({ value: v, label: v }));
    const used = new Set(sessions.map((s) => s.integrationId));
    const list: { key: Facet; label: string; icon: React.ReactNode; options: Option[]; min: number }[] = [
      { key: "clouds", label: "Cloud", icon: <Cloud />, options: clouds, min: 2 },
      {
        key: "integrations",
        label: "Integration",
        icon: <Server />,
        options: integrations.filter((i) => used.has(i.id)).map((i) => ({ value: i.id, label: i.alias })),
        min: 1,
      },
      { key: "kinds", label: "Type", icon: <Bookmark />, options: kinds, min: 2 },
      { key: "regions", label: "Region", icon: <Globe />, options: regions, min: 2 },
      {
        key: "tags",
        label: "Tag",
        icon: <TagIcon />,
        options: tags.map((t) => ({ value: t.name, label: t.name })),
        min: 1,
      },
      { key: "states", label: "State", icon: <Timer />, options: states, min: 2 },
    ];
    return list.filter((f) => f.options.length >= f.min);
  }, [sessions, integrations, tags]);

  const toggle = (key: Facet, v: string, on: boolean) =>
    onChange({ ...value, [key]: on ? [...value[key], v] : value[key].filter((x) => x !== v) });

  return (
    <div role="toolbar" aria-label="Filters" className="flex flex-wrap items-center gap-2 border-b px-5 py-2">
      {facets.map((f) => {
        const picked = value[f.key];
        // One pick names itself; more show a count.
        const text =
          picked.length === 0
            ? f.label
            : picked.length === 1
              ? (f.options.find((o) => o.value === picked[0])?.label ?? picked[0])
              : `${f.label} · ${picked.length}`;
        return (
          <DropdownMenu key={f.key}>
            <DropdownMenuTrigger asChild>
              <Button
                variant={picked.length > 0 ? "default" : "secondary"}
                size="sm"
                className={cn("h-7 gap-1.5 rounded-full px-3 text-xs", picked.length === 0 && "text-muted-foreground")}
                aria-label={`Filter by ${f.label.toLowerCase()}`}
                data-picked={picked.length}
              >
                <span className="[&>svg]:size-3.5">{f.icon}</span>
                {text}
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" className="min-w-44">
              {f.options.map((o) => (
                <DropdownMenuCheckboxItem
                  key={o.value}
                  checked={picked.includes(o.value)}
                  onCheckedChange={(on) => toggle(f.key, o.value, on)}
                  // The menu stays open, so several values toggle in one go.
                  onSelect={(e) => e.preventDefault()}
                >
                  {o.label}
                </DropdownMenuCheckboxItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        );
      })}
      {facets.length === 0 && <p className="text-xs text-muted-foreground">Nothing to filter by yet.</p>}
      {refineCount(value) > 0 && (
        <Button
          variant="ghost"
          size="sm"
          className="h-7 gap-1 px-2 text-xs text-muted-foreground"
          onClick={() => onChange(EMPTY_REFINE)}
        >
          <X className="size-3.5" /> Clear
        </Button>
      )}
    </div>
  );
}
