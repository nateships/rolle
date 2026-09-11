import { Select, SelectContent, SelectGroup, SelectItem, SelectLabel, SelectTrigger, SelectValue } from "@/components/ui/select";
import { AWS_REGIONS } from "@/lib/aws-regions";

/** Picker over the known AWS regions. Shows the region code with its city for scanning. */
export function RegionSelect({ value, onChange, className }: { value: string; onChange: (v: string) => void; className?: string }) {
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger className={className ?? "w-full"}><SelectValue placeholder="Choose a region" /></SelectTrigger>
      <SelectContent className="max-h-72">
        {AWS_REGIONS.map((g) => (
          <SelectGroup key={g.group}>
            <SelectLabel>{g.group}</SelectLabel>
            {g.regions.map((r) => (
              <SelectItem key={r.id} value={r.id}>
                <span className="font-mono text-xs">{r.id}</span>
                <span className="text-muted-foreground">{r.name}</span>
              </SelectItem>
            ))}
          </SelectGroup>
        ))}
      </SelectContent>
    </Select>
  );
}
