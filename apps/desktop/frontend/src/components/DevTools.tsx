import { useEffect, useState } from "react";
import { Bug, RotateCcw, Trash2, FlaskConical } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { api, errorMessage } from "@/lib/api";

/** Inline dev menu for layout footers. Only rendered when the backend was built without the production tag. */
export function DevTools() {
  const [dev, setDev] = useState(false);
  const [demo, setDemo] = useState(false);
  // ?shot=1 hides the dev pill in the browser preview, for screenshots.
  const hidden = new URLSearchParams(location.search).get("shot") === "1";
  useEffect(() => {
    api
      .DevMode()
      .then(setDev)
      .catch(() => setDev(false));
    api
      .DemoMode()
      .then(setDemo)
      .catch(() => setDemo(false));
  }, []);
  if (!dev || hidden) return null;

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
          <Button
            size="sm"
            variant="outline"
            className="h-7 gap-1.5 border-dashed px-2 font-mono text-[11px] uppercase tracking-wider text-muted-foreground"
          >
            {demo ? <FlaskConical className="size-3.5 text-brand-orange" /> : <Bug className="size-3.5" />}{" "}
            {demo ? "demo" : "dev"}
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" side="top">
          <DropdownMenuLabel className="text-xs text-muted-foreground">
            {demo ? "Development build · fictional data" : "Development build"}
          </DropdownMenuLabel>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            onClick={run(demo ? "Relaunching on your real data" : "Relaunching on demo data", () =>
              api.Relaunch(!demo),
            )}
          >
            <FlaskConical /> {demo ? "Relaunch with real data" : "Relaunch with demo data"}
          </DropdownMenuItem>
          <DropdownMenuItem onClick={run("Onboarding will replay", () => api.ReplayOnboarding())}>
            <RotateCcw /> Replay onboarding
          </DropdownMenuItem>
          <DropdownMenuItem variant="destructive" onClick={run("Workspace reset", () => api.Reset())}>
            <Trash2 /> Reset workspace
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
