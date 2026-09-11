import { useEffect, useState } from "react";
import { Check, Copy, Loader2, Monitor, Moon, RotateCcw, Sun, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Switch } from "@/components/ui/switch";
import { RegionSelect } from "@/components/RegionSelect";
import { api, errorMessage } from "@/lib/api";
import { copyText } from "@/lib/clipboard";
import { applyTheme, type Theme } from "@/lib/theme";
import { cn } from "@/lib/utils";

type Settings = { theme: string; defaultRegion: string; assumeRoleMinutes: number; hideOnClose: boolean; verboseLogging: boolean };
type Info = { version: string; workspacePath: string; cacheDir: string; awsConfigPath: string };

const DURATIONS = [60, 120, 240, 480, 720];

export function SettingsDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [settings, setSettings] = useState<Settings | null>(null);
  const [info, setInfo] = useState<Info | null>(null);
  const [saving, setSaving] = useState(false);
  const [confirmReset, setConfirmReset] = useState(false);

  useEffect(() => {
    if (!open) { setConfirmReset(false); return; }
    api.Settings().then((s) => setSettings(s as Settings)).catch((e) => toast.error(errorMessage(e)));
    api.Info().then((i) => setInfo(i as Info)).catch(() => setInfo(null));
  }, [open]);

  const update = async (patch: Partial<Settings>) => {
    if (!settings) return;
    const next = { ...settings, ...patch };
    setSettings(next);
    if (patch.theme) applyTheme(patch.theme as Theme);
    setSaving(true);
    try {
      const saved = (await api.UpdateSettings(next)) as Settings;
      setSettings(saved);
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setSaving(false);
    }
  };

  const run = (label: string, fn: () => Promise<unknown>) => async () => {
    try {
      await fn();
      toast.success(label);
      onClose();
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[85vh] max-w-lg overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">Settings {saving && <Loader2 className="size-3.5 animate-spin text-muted-foreground" />}</DialogTitle>
          <DialogDescription>Preferences are saved as you change them.</DialogDescription>
        </DialogHeader>
        {settings && (
          <div className="space-y-6">
            <section className="space-y-4">
              <Row label="Appearance" hint="Follow the system or pick one.">
                <div className="inline-flex rounded-md border bg-muted p-0.5">
                  {([["system", Monitor, "System"], ["light", Sun, "Light"], ["dark", Moon, "Dark"]] as const).map(([value, Icon, label]) => (
                    <button
                      key={value}
                      type="button"
                      onClick={() => update({ theme: value })}
                      className={cn("flex items-center gap-1.5 rounded-sm px-2.5 py-1 text-xs transition-colors", settings.theme === value ? "bg-background text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground")}
                    >
                      <Icon className="size-3.5" /> {label}
                    </button>
                  ))}
                </div>
              </Row>
              <Row label="Default AWS region" hint="Pre-filled for new sessions.">
                <RegionSelect value={settings.defaultRegion} onChange={(v) => update({ defaultRegion: v })} className="w-56 justify-between font-normal" />
              </Row>
              <Row label="Assume role duration" hint="Requested from STS. Roles may cap it lower.">
                <Select value={String(settings.assumeRoleMinutes)} onValueChange={(v) => update({ assumeRoleMinutes: Number(v) })}>
                  <SelectTrigger className="w-56"><SelectValue /></SelectTrigger>
                  <SelectContent>{DURATIONS.map((m) => <SelectItem key={m} value={String(m)}>{m >= 60 ? `${m / 60} hour${m === 60 ? "" : "s"}` : `${m} minutes`}</SelectItem>)}</SelectContent>
                </Select>
              </Row>
              <Row label="Keep running in the tray" hint="Closing the window hides it instead of quitting.">
                <Switch checked={settings.hideOnClose} onCheckedChange={(v) => update({ hideOnClose: v })} />
              </Row>
              <Row label="Verbose logging" hint="Same as ROLLE_DEBUG=1. Prints diagnostics to the app log.">
                <Switch checked={settings.verboseLogging} onCheckedChange={(v) => update({ verboseLogging: v })} />
              </Row>
            </section>

            <Separator />

            <section className="space-y-2 text-xs">
              <p className="font-medium text-foreground">About</p>
              <PathRow label="Version" value={info?.version ?? "…"} />
              <PathRow label="Workspace" value={info?.workspacePath ?? "…"} copy />
              <PathRow label="Credential cache" value={info?.cacheDir ?? "…"} copy />
              <PathRow label="AWS config" value={info?.awsConfigPath ?? "…"} copy />
            </section>

            <Separator />

            <section className="space-y-3">
              <p className="text-xs font-medium text-destructive">Danger zone</p>
              <div className="flex flex-wrap gap-2">
                <Button variant="secondary" size="sm" className="gap-1.5" onClick={run("Onboarding will replay", () => api.ReplayOnboarding())}>
                  <RotateCcw className="size-3.5" /> Replay onboarding
                </Button>
                {confirmReset ? (
                  <Button variant="destructive" size="sm" className="gap-1.5" onClick={run("Rolle was reset", () => api.Reset())}>
                    <Check className="size-3.5" /> Yes, remove everything
                  </Button>
                ) : (
                  <Button variant="outline" size="sm" className="gap-1.5 border-destructive/40 text-destructive hover:bg-destructive/10" onClick={() => setConfirmReset(true)}>
                    <Trash2 className="size-3.5" /> Reset Rolle…
                  </Button>
                )}
              </div>
              <p className="text-[11px] text-muted-foreground">Reset removes every session, integration, keychain secret, cached credential, and Rolle-owned AWS profile.</p>
            </section>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}

function Row({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4">
      <div className="min-w-0">
        <Label className="text-sm">{label}</Label>
        {hint && <p className="text-[11px] text-muted-foreground">{hint}</p>}
      </div>
      <div className="shrink-0">{children}</div>
    </div>
  );
}

function PathRow({ label, value, copy }: { label: string; value: string; copy?: boolean }) {
  return (
    <div className="flex items-center gap-2">
      <span className="w-28 shrink-0 text-muted-foreground">{label}</span>
      <code className="min-w-0 flex-1 truncate font-mono text-[11px]">{value}</code>
      {copy && (
        <Button variant="ghost" size="icon-xs" aria-label={`Copy ${label}`} onClick={() => copyText(value).then(() => toast.success(`${label} copied`)).catch((e) => toast.error(errorMessage(e)))}>
          <Copy className="size-3" />
        </Button>
      )}
    </div>
  );
}
