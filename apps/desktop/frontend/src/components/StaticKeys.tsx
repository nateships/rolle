import { useEffect, useState } from "react";
import { KeyRound, Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { api, errorMessage } from "@/lib/api";
import { cn } from "@/lib/utils";

type Keys = { path: string; profiles: string[] };

/**
 * The sections of ~/.aws/credentials that hold static keys. Tools read those
 * before any rolle profile of the same name, so the card offers to remove
 * them. It renders nothing when the file holds none. Removal is a two-step
 * click: the first arms the button, the second removes.
 */
export function StaticKeysCard({ className }: { className?: string }) {
  const [keys, setKeys] = useState<Keys | null>(null);
  const [armed, setArmed] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const refresh = () =>
    void api
      .StaticProfiles()
      .then((k) => setKeys({ path: k.path, profiles: k.profiles ?? [] }))
      .catch((e) => toast.error(errorMessage(e)));
  useEffect(refresh, []);
  // An armed button disarms on its own.
  useEffect(() => {
    if (!armed) return;
    const t = setTimeout(() => setArmed(null), 4000);
    return () => clearTimeout(t);
  }, [armed]);

  if (!keys || keys.profiles.length === 0) return null;

  async function remove(names: string[]) {
    setBusy(true);
    setArmed(null);
    try {
      for (const n of names) await api.RemoveStaticProfile(n);
      toast.success(names.length === 1 ? `Static keys of ${names[0]} removed` : "Static keys removed");
      refresh();
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }
  const removeButton = (key: string, label: string, names: string[]) => (
    <Button
      size="sm"
      variant={armed === key ? "destructive" : "ghost"}
      className="h-7"
      disabled={busy}
      onClick={() => (armed === key ? void remove(names) : setArmed(key))}
    >
      {busy && armed === null ? <Loader2 className="size-3.5 animate-spin" /> : armed === key ? "Confirm" : label}
    </Button>
  );

  return (
    <div className={cn("rounded-lg border bg-card p-4 text-left text-sm", className)}>
      <div className="flex items-start gap-3">
        <KeyRound className="mt-0.5 size-4 shrink-0 text-amber-500" />
        <div className="min-w-0 flex-1">
          <p className="font-medium">Static keys in {keys.path}</p>
          <p className="mt-0.5 text-xs text-muted-foreground">
            Tools read these before a rolle profile of the same name.
          </p>
          <ul className="mt-3 max-h-40 space-y-1 overflow-y-auto pr-1">
            {keys.profiles.map((p) => (
              <li key={p} className="flex items-center justify-between gap-3">
                <code className="font-mono text-xs">{p}</code>
                {removeButton(p, "Remove", [p])}
              </li>
            ))}
          </ul>
          {keys.profiles.length > 1 && (
            <div className="mt-2 flex justify-end">{removeButton("*", "Remove all", keys.profiles)}</div>
          )}
        </div>
      </div>
    </div>
  );
}
