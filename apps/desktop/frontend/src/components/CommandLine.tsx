import { useEffect, useState } from "react";
import { Check, Loader2, TerminalSquare } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { api, errorMessage, type CLIStatus } from "@/lib/api";
import { cn } from "@/lib/utils";

/**
 * Status and install control for the rolle command that ships inside the app.
 * Renders nothing where the command cannot be linked (dev builds, other OSes).
 */
export function CommandLineInstall({ compact = false, className }: { compact?: boolean; className?: string }) {
  const [status, setStatus] = useState<CLIStatus | null>(null);
  const [busy, setBusy] = useState(false);
  const refresh = () => void api.CLIStatus().then(setStatus);
  useEffect(refresh, []);

  if (!status || status.reason === "unsupported") return null;

  async function run(action: () => Promise<unknown>, done: string) {
    setBusy(true);
    try {
      await action();
      toast.success(done);
      refresh();
    } catch (e) {
      const msg = errorMessage(e);
      if (msg !== "cancelled") toast.error(msg);
    } finally {
      setBusy(false);
    }
  }

  const install = () => run(() => api.InstallCLI(), "rolle command installed");
  const remove = () => run(() => api.UninstallCLI(), "rolle command removed");

  if (compact) {
    return status.installed ? (
      <div className={cn("flex items-center gap-2 text-xs", className)}>
        <span className="flex items-center gap-1 text-emerald-600 dark:text-emerald-400">
          <Check className="size-3.5" /> Installed
        </span>
        <Button size="sm" variant="ghost" className="h-7" onClick={remove} disabled={busy}>
          Remove
        </Button>
      </div>
    ) : (
      <Button
        size="sm"
        variant="secondary"
        className={cn("gap-1.5", className)}
        onClick={install}
        disabled={busy || status.reason === "move"}
      >
        {busy ? <Loader2 className="size-3.5 animate-spin" /> : <TerminalSquare className="size-3.5" />} Install command
      </Button>
    );
  }

  return (
    <div
      className={cn(
        "flex w-full items-center justify-between gap-4 rounded-lg border bg-card px-4 py-3 text-left",
        className,
      )}
    >
      <div className="min-w-0">
        <p className="text-sm font-medium">
          The <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs">rolle</code> command
        </p>
        <p className="text-xs text-muted-foreground">
          {status.installed
            ? `Ready in your terminal at ${status.path}.`
            : status.reason === "move"
              ? "Move Rolle to the Applications folder, then install the command from Settings."
              : "Ships inside the app. Link it into /usr/local/bin for every terminal."}
        </p>
      </div>
      {status.installed ? (
        <span className="flex shrink-0 items-center gap-1 text-xs text-emerald-600 dark:text-emerald-400">
          <Check className="size-4" /> Installed
        </span>
      ) : (
        <Button size="sm" className="shrink-0 gap-1.5" onClick={install} disabled={busy || status.reason === "move"}>
          {busy ? <Loader2 className="size-3.5 animate-spin" /> : <TerminalSquare className="size-3.5" />} Install
          command
        </Button>
      )}
    </div>
  );
}
