import { useEffect, useState } from "react";
import { Check, Copy, Download, Loader2, Monitor, Moon, RefreshCw, RotateCcw, Sun, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Switch } from "@/components/ui/switch";
import { RegionSelect } from "@/components/RegionSelect";
import { api, errorMessage, type AppInfo, type Settings, type UpdateInfo } from "@/lib/api";
import { copyText } from "@/lib/clipboard";
import { applyTheme, type Theme } from "@/lib/theme";
import { cn } from "@/lib/utils";

const IS_MAC = /Macintosh/.test(navigator.userAgent);
const IS_WIN = /Windows/.test(navigator.userAgent);
const TERMINALS: { value: string; label: string }[] = IS_MAC
  ? [
      { value: "auto", label: "Detect (cmux, Ghostty, iTerm, Warp, Terminal)" },
      { value: "cmux", label: "cmux" },
      { value: "ghostty", label: "Ghostty" },
      { value: "iterm", label: "iTerm2" },
      { value: "warp", label: "Warp" },
      { value: "terminal", label: "Terminal" },
    ]
  : IS_WIN
    ? [
        { value: "auto", label: "Windows Terminal if installed" },
        { value: "powershell", label: "PowerShell window" },
      ]
    : [{ value: "auto", label: "$TERMINAL or the system default" }];

const DURATIONS = [60, 120, 240, 480, 720];

export function SettingsDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [settings, setSettings] = useState<Settings | null>(null);
  const [info, setInfo] = useState<AppInfo | null>(null);
  const [saving, setSaving] = useState(false);
  const [confirmReset, setConfirmReset] = useState(false);
  const [updateInfo, setUpdateInfo] = useState<UpdateInfo | null>(null);
  const [checking, setChecking] = useState(false);
  const [installing, setInstalling] = useState(false);

  const checkUpdates = async () => {
    setChecking(true);
    try {
      const update = await api.CheckForUpdates();
      setUpdateInfo(update);
      if (!update.enabled) toast.info("Updates are disabled in development builds");
      else if (update.available) toast.success(`Rolle ${update.version} is available`);
      else toast.success("You're on the latest version");
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setChecking(false);
    }
  };

  const installUpdate = async () => {
    setInstalling(true);
    try {
      await api.InstallUpdate();
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setInstalling(false);
    }
  };

  useEffect(() => {
    if (!open) {
      setConfirmReset(false);
      return;
    }
    api
      .Settings()
      .then(setSettings)
      .catch((e) => toast.error(errorMessage(e)));
    api
      .Info()
      .then(setInfo)
      .catch(() => setInfo(null));
  }, [open]);

  const update = async (patch: Partial<Settings>) => {
    if (!settings) return;
    const next = { ...settings, ...patch };
    setSettings(next);
    if (patch.theme) applyTheme(patch.theme as Theme);
    setSaving(true);
    try {
      setSettings(await api.UpdateSettings(next));
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
      <DialogContent className="overflow-hidden sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            Settings {saving && <Loader2 className="size-3.5 animate-spin text-muted-foreground" />}
          </DialogTitle>
          <DialogDescription>Preferences are saved as you change them.</DialogDescription>
        </DialogHeader>
        {settings && (
          <Tabs defaultValue={new URLSearchParams(location.search).get("tab") ?? "general"} className="w-full">
            <TabsList className="grid w-full grid-cols-4">
              <TabsTrigger value="general">General</TabsTrigger>
              <TabsTrigger value="appearance">Appearance</TabsTrigger>
              <TabsTrigger value="about">About</TabsTrigger>
              <TabsTrigger value="advanced">Advanced</TabsTrigger>
            </TabsList>

            <TabsContent value="general" className="mt-4 min-h-64 space-y-4">
              <Row label="Default AWS region" hint="Pre-filled for new sessions.">
                <RegionSelect
                  value={settings.defaultRegion}
                  onChange={(v) => update({ defaultRegion: v })}
                  className="w-64 justify-between font-normal"
                />
              </Row>
              <Row label="Assume role duration" hint="Requested from STS. Roles may cap it lower.">
                <Select
                  value={String(settings.assumeRoleMinutes)}
                  onValueChange={(v) => update({ assumeRoleMinutes: Number(v) })}
                >
                  <SelectTrigger className="w-64">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {DURATIONS.map((m) => (
                      <SelectItem key={m} value={String(m)}>
                        {m >= 60 ? `${m / 60} hour${m === 60 ? "" : "s"}` : `${m} minutes`}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Row>
              <Row label="Terminal app" hint="Used by Open terminal on a session.">
                <Select value={settings.terminal || "auto"} onValueChange={(v) => update({ terminal: v })}>
                  <SelectTrigger className="w-64">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {TERMINALS.map((t) => (
                      <SelectItem key={t.value} value={t.value}>
                        {t.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Row>
              <Row label="Keep running in the tray" hint="Closing the window hides it instead of quitting.">
                <Switch checked={settings.hideOnClose} onCheckedChange={(v) => update({ hideOnClose: v })} />
              </Row>
              <Row label="Verbose logging" hint="Same as ROLLE_DEBUG=1. Prints diagnostics to the app log.">
                <Switch checked={settings.verboseLogging} onCheckedChange={(v) => update({ verboseLogging: v })} />
              </Row>
              <Row
                label="Automatic updates"
                hint="Check for new releases every few hours. Takes effect on next launch."
              >
                <Switch checked={!settings.autoUpdateOff} onCheckedChange={(v) => update({ autoUpdateOff: !v })} />
              </Row>
            </TabsContent>

            <TabsContent value="appearance" className="mt-4 min-h-64 space-y-4">
              <Row label="Theme" hint="Follow the system or pick one.">
                <div className="inline-flex rounded-md border bg-muted p-0.5">
                  {(
                    [
                      ["system", Monitor, "System"],
                      ["light", Sun, "Light"],
                      ["dark", Moon, "Dark"],
                    ] as const
                  ).map(([value, Icon, label]) => (
                    <button
                      key={value}
                      type="button"
                      onClick={() => update({ theme: value })}
                      className={cn(
                        "flex items-center gap-1.5 rounded-sm px-3 py-1.5 text-xs transition-colors",
                        settings.theme === value
                          ? "bg-background text-foreground shadow-sm"
                          : "text-muted-foreground hover:text-foreground",
                      )}
                    >
                      <Icon className="size-3.5" /> {label}
                    </button>
                  ))}
                </div>
              </Row>
            </TabsContent>

            <TabsContent value="about" className="mt-4 min-h-64 space-y-2 text-xs">
              <div className="mb-3 flex items-center justify-between rounded-lg border bg-card px-3 py-2">
                <div>
                  <p className="text-sm font-medium text-foreground">Rolle {info?.version ?? "…"}</p>
                  <p className="text-[11px] text-muted-foreground">
                    {updateInfo === null
                      ? "Updates are signed and verified before they install."
                      : !updateInfo.enabled
                        ? "Development build, updates disabled."
                        : updateInfo.available
                          ? `Version ${updateInfo.version} is ready to install.`
                          : "You're on the latest version."}
                  </p>
                </div>
                {updateInfo?.available ? (
                  <Button size="sm" className="gap-1.5" onClick={installUpdate} disabled={installing}>
                    {installing ? <Loader2 className="size-3.5 animate-spin" /> : <Download className="size-3.5" />}{" "}
                    Install {updateInfo.version}
                  </Button>
                ) : (
                  <Button size="sm" variant="secondary" className="gap-1.5" onClick={checkUpdates} disabled={checking}>
                    {checking ? <Loader2 className="size-3.5 animate-spin" /> : <RefreshCw className="size-3.5" />}{" "}
                    Check for updates
                  </Button>
                )}
              </div>
              <PathRow label="Version" value={info?.version ?? "…"} />
              <PathRow label="Workspace" value={info?.workspacePath ?? "…"} copy />
              <PathRow label="Credential cache" value={info?.cacheDir ?? "…"} copy />
              <PathRow label="AWS config" value={info?.awsConfigPath ?? "…"} copy />
              <p className="pt-3 text-[11px] text-muted-foreground">
                Secrets live in the OS keychain. Short-lived credentials are cached with owner-only permissions and
                expire on their own.
              </p>
            </TabsContent>

            <TabsContent value="advanced" className="mt-4 min-h-64 space-y-3">
              <p className="text-xs font-medium text-destructive">Danger zone</p>
              <div className="flex flex-wrap gap-2">
                <Button
                  variant="secondary"
                  size="sm"
                  className="gap-1.5"
                  onClick={run("Onboarding will replay", () => api.ReplayOnboarding())}
                >
                  <RotateCcw className="size-3.5" /> Replay onboarding
                </Button>
                {confirmReset ? (
                  <Button
                    variant="destructive"
                    size="sm"
                    className="gap-1.5"
                    onClick={run("Rolle was reset", () => api.Reset())}
                  >
                    <Check className="size-3.5" /> Yes, remove everything
                  </Button>
                ) : (
                  <Button
                    variant="outline"
                    size="sm"
                    className="gap-1.5 border-destructive/40 text-destructive hover:bg-destructive/10"
                    onClick={() => setConfirmReset(true)}
                  >
                    <Trash2 className="size-3.5" /> Reset Rolle…
                  </Button>
                )}
              </div>
              <p className="text-[11px] text-muted-foreground">
                Reset removes every session, integration, keychain secret, cached credential, and Rolle-owned AWS
                profile.
              </p>
            </TabsContent>
          </Tabs>
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
      <code className="min-w-0 flex-1 truncate font-mono text-[11px]" title={value}>
        {value}
      </code>
      {copy && (
        <Button
          variant="ghost"
          size="icon-xs"
          aria-label={`Copy ${label}`}
          onClick={() =>
            copyText(value)
              .then(() => toast.success(`${label} copied`))
              .catch((e) => toast.error(errorMessage(e)))
          }
        >
          <Copy className="size-3" />
        </Button>
      )}
    </div>
  );
}
