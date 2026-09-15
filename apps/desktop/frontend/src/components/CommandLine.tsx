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
  const refresh = () =>
    void api
      .CLIStatus()
      .then(setStatus)
      .catch((e) => toast.error(errorMessage(e)));
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
  const outdated = status.reason === "outdated";
  // A command from Homebrew or a package: shown, never replaced or removed.
  const external = status.reason === "external";
  const actionLabel = outdated ? "Update command" : "Install command";

  if (compact) {
    return status.installed && !outdated ? (
      <div className={cn("flex items-center gap-2 text-xs", className)}>
        <span className="flex items-center gap-1 text-emerald-600 dark:text-emerald-400">
          <Check className="size-3.5" /> Installed
        </span>
        {!external && (
          <Button size="sm" variant="ghost" className="h-7" onClick={remove} disabled={busy}>
            Remove
          </Button>
        )}
      </div>
    ) : (
      <Button
        size="sm"
        variant="secondary"
        className={cn("gap-1.5", className)}
        onClick={install}
        disabled={busy || status.reason === "move"}
      >
        {busy ? <Loader2 className="size-3.5 animate-spin" /> : <TerminalSquare className="size-3.5" />} {actionLabel}
      </Button>
    );
  }

  return (
    <div className={cn("flex w-full items-center gap-4 rounded-xl border bg-card p-4 text-left", className)}>
      <div className="shrink-0 rounded-lg bg-muted p-2.5">
        <TerminalSquare className="size-5" />
      </div>
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium">Use rolle from your terminal</p>
        <p className="mt-0.5 text-xs text-muted-foreground">
          {outdated ? (
            <>
              The command at <span className="font-mono text-[11px]">{status.path}</span> is from another version.
            </>
          ) : status.installed ? (
            <>
              <code className="rounded bg-muted px-1 py-px font-mono text-[11px]">rolle</code> is on your PATH at{" "}
              <span className="font-mono text-[11px]">{status.path}</span>
              {external ? ", installed another way." : "."}
            </>
          ) : status.reason === "move" ? (
            "Move rolle to Applications first."
          ) : (
            <>
              Adds <code className="rounded bg-muted px-1 py-px font-mono text-[11px]">rolle</code> to your PATH.
            </>
          )}
          {status.note ? <> {status.note}</> : null}
        </p>
      </div>
      {status.installed && !outdated ? (
        <span className="flex shrink-0 items-center gap-1 text-xs text-emerald-600 dark:text-emerald-400">
          <Check className="size-4" /> Installed
        </span>
      ) : (
        <Button size="sm" className="shrink-0 gap-1.5" onClick={install} disabled={busy || status.reason === "move"}>
          {busy ? <Loader2 className="size-3.5 animate-spin" /> : <TerminalSquare className="size-3.5" />} {actionLabel}
        </Button>
      )}
    </div>
  );
}
