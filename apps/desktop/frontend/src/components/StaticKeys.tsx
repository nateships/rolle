import { useEffect, useState } from "react";
import { KeyRound } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { REMOVE_KEYS_BUTTON, RemoveKeysDialog, type RemoveKeysTarget } from "@/components/dialogs/RemoveKeysDialog";
import { api, errorMessage } from "@/lib/api";
import { cn } from "@/lib/utils";

type Keys = { path: string; profiles: { name: string; imported: boolean }[] };

/**
 * The sections of ~/.aws/credentials that hold static keys. Tools read those
 * before any rolle profile of the same name, so the card offers to move each
 * key into rolle or remove it. It renders nothing when the file holds none.
 * Every removal goes through the shared confirmation.
 */
export function StaticKeysCard({ className }: { className?: string }) {
  const [keys, setKeys] = useState<Keys | null>(null);
  const [removing, setRemoving] = useState<RemoveKeysTarget | null>(null);
  const [importing, setImporting] = useState<string | null>(null);
  const refresh = () =>
    void api
      .StaticProfiles()
      .then((k) =>
        setKeys({ path: k.path, profiles: (k.profiles ?? []).map((p) => ({ name: p.name, imported: !!p.imported })) }),
      )
      .catch((e) => toast.error(errorMessage(e)));
  useEffect(refresh, []);

  // The key moves into rolle, then the dialog offers to remove it from the file.
  async function importKey(name: string) {
    setImporting(name);
    try {
      await api.ImportIAMUser(name);
      toast.success("Imported", { description: name });
      refresh();
      setRemoving({ profiles: [name] });
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setImporting(null);
    }
  }

  if (!keys || keys.profiles.length === 0) return null;

  const removeButton = (label: string, profiles: string[]) => (
    <Button
      size="sm"
      variant="outline"
      className={cn("h-7", REMOVE_KEYS_BUTTON)}
      onClick={() => setRemoving({ profiles })}
    >
      {label}
    </Button>
  );

  return (
    <div className={cn("rounded-lg border bg-card p-4 text-left text-sm", className)}>
      <div className="flex items-start gap-3">
        <KeyRound className="mt-0.5 size-4 shrink-0 text-amber-500" />
        <div className="min-w-0 flex-1">
          <p className="font-medium">Profiles in {keys.path}</p>
          <p className="mt-0.5 text-xs text-muted-foreground">Import moves a key into rolle. Remove deletes it.</p>
          <ul className="mt-3 max-h-40 space-y-1 overflow-y-auto pr-1">
            {keys.profiles.map((p) => (
              <li key={p.name} className="flex items-center justify-between gap-3">
                <code className="font-mono text-xs">{p.name}</code>
                <div className="flex gap-1.5">
                  {!p.imported && (
                    <Button
                      size="sm"
                      variant="secondary"
                      className="h-7"
                      onClick={() => void importKey(p.name)}
                      disabled={importing !== null}
                    >
                      Import
                    </Button>
                  )}
                  {removeButton("Remove", [p.name])}
                </div>
              </li>
            ))}
          </ul>
          {keys.profiles.length > 1 && (
            <div className="mt-2 flex justify-end">
              {removeButton(
                "Remove all",
                keys.profiles.map((p) => p.name),
              )}
            </div>
          )}
        </div>
      </div>
      <RemoveKeysDialog target={removing} onClose={() => setRemoving(null)} onDone={refresh} />
    </div>
  );
}
