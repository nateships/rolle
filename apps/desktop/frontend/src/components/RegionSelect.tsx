import { useMemo, useState } from "react";
import { Check, ChevronsUpDown } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "@/components/ui/command";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { AWS_REGIONS } from "@/lib/aws-regions";
import { cn } from "@/lib/utils";

/** Searchable picker over the known AWS regions. Matches on code, city, or group. */
export function RegionSelect({ value, onChange, className }: { value: string; onChange: (v: string) => void; className?: string }) {
  const [open, setOpen] = useState(false);
  const selected = useMemo(() => AWS_REGIONS.flatMap((g) => g.regions).find((r) => r.id === value), [value]);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button type="button" variant="outline" role="combobox" aria-expanded={open} className={cn("w-full justify-between font-normal", className)}>
          {selected ? (
            <span className="flex items-center gap-2 truncate"><span className="font-mono text-xs">{selected.id}</span><span className="text-muted-foreground">{selected.name}</span></span>
          ) : (
            <span className="text-muted-foreground">Choose a region</span>
          )}
          <ChevronsUpDown className="size-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-(--radix-popover-trigger-width) p-0" align="start">
        <Command>
          <CommandInput placeholder="Search regions…" autoFocus />
          <CommandList className="max-h-72">
            <CommandEmpty>No region matches.</CommandEmpty>
            {AWS_REGIONS.map((g) => (
              <CommandGroup key={g.group} heading={g.group}>
                {g.regions.map((r) => (
                  <CommandItem
                    key={r.id}
                    value={`${r.id} ${r.name} ${g.group}`}
                    onSelect={() => { onChange(r.id); setOpen(false); }}
                  >
                    <span className="font-mono text-xs">{r.id}</span>
                    <span className="text-muted-foreground">{r.name}</span>
                    <Check className={cn("ml-auto size-4", r.id === value ? "opacity-100" : "opacity-0")} />
                  </CommandItem>
                ))}
              </CommandGroup>
            ))}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
