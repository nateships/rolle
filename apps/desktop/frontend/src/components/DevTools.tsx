import { useEffect, useState } from "react";
import { Bug, RotateCcw, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { api, errorMessage } from "@/lib/api";

/** Inline dev menu for layout footers. Only rendered when the backend was built without the production tag. */
export function DevTools() {
  const [dev, setDev] = useState(false);
  useEffect(() => {
    api.DevMode().then(setDev).catch(() => setDev(false));
  }, []);
  if (!dev) return null;

  const run = (label: string, fn: () => Promise<unknown>) => async () => {
    try {
      await fn();
      toast.success(label);
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };

  return (
    <div className="no-drag">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button size="sm" variant="outline" className="h-7 gap-1.5 border-dashed px-2 font-mono text-[11px] uppercase tracking-wider text-muted-foreground">
            <Bug className="size-3.5" /> dev
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" side="top">
          <DropdownMenuLabel className="text-xs text-muted-foreground">Development build</DropdownMenuLabel>
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={run("Onboarding will replay", () => api.DevReplayOnboarding())}><RotateCcw /> Replay onboarding</DropdownMenuItem>
          <DropdownMenuItem variant="destructive" onClick={run("Workspace reset", () => api.DevReset())}><Trash2 /> Reset workspace</DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
