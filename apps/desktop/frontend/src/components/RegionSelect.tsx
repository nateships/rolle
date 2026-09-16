import { useMemo, useRef } from "react";
import { Combobox } from "@base-ui/react/combobox";
import { Check, ChevronsUpDown, Search } from "lucide-react";
import { buttonVariants } from "@/components/ui/button";
import { AWS_REGIONS } from "@/lib/aws-regions";
import { cn } from "@/lib/utils";

type Region = { id: string; name: string; group: string };
type Group = { value: string; items: Region[] };

// One item per region, grouped the way the data is. The label carries the
// code, the city, and the group, so typing any of them finds the region.
const GROUPS: Group[] = AWS_REGIONS.map((g) => ({
  value: g.group,
  items: g.regions.map((r) => ({ ...r, group: g.group })),
}));
const REGIONS = GROUPS.flatMap((g) => g.items);
const label = (r: Region) => `${r.id} ${r.name} ${r.group}`;

/** Searchable picker over the known AWS regions. Matches on code, city, or group. */
export function RegionSelect({
  value,
  onChange,
  className,
}: {
  value: string;
  onChange: (v: string) => void;
  className?: string;
}) {
  const selected = useMemo(() => REGIONS.find((r) => r.id === value) ?? null, [value]);
  // The popup mounts here, next to the trigger, instead of on the body. This picker
  // opens inside a Radix dialog. The dialog traps focus in its own DOM and pulls it
  // back from a popup on the body, so the search input never keeps focus there.
  const container = useRef<HTMLDivElement>(null);
  return (
    <Combobox.Root<Region>
      items={GROUPS}
      value={selected}
      onValueChange={(r) => r && onChange(r.id)}
      itemToStringLabel={label}
      autoHighlight
    >
      <Combobox.Trigger
        className={cn(buttonVariants({ variant: "outline" }), "w-full justify-between font-normal", className)}
      >
        {selected ? (
          <span className="flex items-center gap-2 truncate">
            <span className="font-mono text-xs">{selected.id}</span>
            <span className="text-muted-foreground">{selected.name}</span>
          </span>
        ) : (
          <span className="text-muted-foreground">Choose a region</span>
        )}
        <ChevronsUpDown className="size-4 shrink-0 opacity-50" />
      </Combobox.Trigger>
      <div ref={container} className="contents" />
      <Combobox.Portal container={container}>
        <Combobox.Positioner align="start" sideOffset={4} className="z-50 outline-none">
          <Combobox.Popup
            aria-label="Regions"
            className="flex w-(--anchor-width) max-h-(--available-height) origin-(--transform-origin) flex-col overflow-hidden rounded-xl bg-popover p-1 text-sm text-popover-foreground shadow-md ring-1 ring-foreground/10 outline-hidden transition-[scale,opacity] duration-100 data-starting-style:scale-95 data-starting-style:opacity-0 data-ending-style:scale-95 data-ending-style:opacity-0"
          >
            <div className="flex items-center gap-2 border-b px-2 pb-1.5">
              <Search className="size-4 shrink-0 opacity-50" />
              <Combobox.Input
                placeholder="Search regions…"
                className="h-8 w-full bg-transparent text-sm outline-hidden placeholder:text-muted-foreground"
              />
            </div>
            <Combobox.Empty className="py-6 text-center text-sm empty:hidden">No region matches.</Combobox.Empty>
            <Combobox.List className="no-scrollbar max-h-72 scroll-py-1 overflow-x-hidden overflow-y-auto outline-none">
              {(group: Group) => (
                <Combobox.Group key={group.value} items={group.items} className="overflow-hidden p-1">
                  <Combobox.GroupLabel className="px-2 py-1.5 text-xs font-medium text-muted-foreground">
                    {group.value}
                  </Combobox.GroupLabel>
                  <Combobox.Collection>
                    {(r: Region) => (
                      <Combobox.Item
                        key={r.id}
                        value={r}
                        className="relative flex cursor-default items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-hidden select-none data-highlighted:bg-muted data-highlighted:text-foreground [&_svg]:pointer-events-none [&_svg]:shrink-0"
                      >
                        <span className="font-mono text-xs">{r.id}</span>
                        <span className="text-muted-foreground">{r.name}</span>
                        <Combobox.ItemIndicator className="ml-auto">
                          <Check className="size-4" />
                        </Combobox.ItemIndicator>
                      </Combobox.Item>
                    )}
                  </Combobox.Collection>
                </Combobox.Group>
              )}
            </Combobox.List>
          </Combobox.Popup>
        </Combobox.Positioner>
      </Combobox.Portal>
    </Combobox.Root>
  );
}
