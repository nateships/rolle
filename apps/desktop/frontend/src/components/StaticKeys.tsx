import { useEffect, useState } from "react";
import { KeyRound } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { REMOVE_KEYS_BUTTON, RemoveKeysDialog, type RemoveKeysTarget } from "@/components/dialogs/RemoveKeysDialog";
import { api, errorMessage } from "@/lib/api";
import { cn } from "@/lib/utils";

type Keys = { path: string; profiles: string[] };

/**
 * The sections of ~/.aws/credentials that hold static keys. Tools read those
 * before any rolle profile of the same name, so the card offers to remove
 * them. It renders nothing when the file holds none. Every removal goes
 * through the shared confirmation.
 */
export function StaticKeysCard({ className }: { className?: string }) {
  const [keys, setKeys] = useState<Keys | null>(null);
  const [removing, setRemoving] = useState<RemoveKeysTarget | null>(null);
  const refresh = () =>
    void api
      .StaticProfiles()
      .then((k) => setKeys({ path: k.path, profiles: k.profiles ?? [] }))
      .catch((e) => toast.error(errorMessage(e)));
  useEffect(refresh, []);

  if (!keys || keys.profiles.length === 0) return null;

  const removeButton = (label: string, profiles: string[]) => (
    <Button
      size="sm"
      variant="outline"
      className={cn("h-7", REMOVE_KEYS_BUTTON)}
      onClick={() => setRemoving({ path: keys.path, profiles })}
    >
      {label}
    </Button>
  );

  return (
    <div className={cn("rounded-lg border bg-card p-4 text-left text-sm", className)}>
      <div className="flex items-start gap-3">
        <KeyRound className="mt-0.5 size-4 shrink-0 text-amber-500" />
        <div className="min-w-0 flex-1">
          <p className="font-medium">Static keys in {keys.path}</p>
          <p className="mt-0.5 text-xs text-muted-foreground">
            Tools read these before a rolle profile with the same name.
          </p>
          <ul className="mt-3 max-h-40 space-y-1 overflow-y-auto pr-1">
            {keys.profiles.map((p) => (
              <li key={p} className="flex items-center justify-between gap-3">
                <code className="font-mono text-xs">{p}</code>
                {removeButton("Remove…", [p])}
              </li>
            ))}
          </ul>
          {keys.profiles.length > 1 && (
            <div className="mt-2 flex justify-end">{removeButton("Remove all…", keys.profiles)}</div>
          )}
        </div>
      </div>
      <RemoveKeysDialog target={removing} onClose={() => setRemoving(null)} onDone={refresh} />
    </div>
  );
}
