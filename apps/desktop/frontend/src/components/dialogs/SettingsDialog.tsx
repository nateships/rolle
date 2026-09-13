import { useEffect, useState } from "react";
import {
  Bug,
  Check,
  Copy,
  Download,
  FileArchive,
  Loader2,
  Monitor,
  Moon,
  RefreshCw,
  RotateCcw,
  Sun,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { StaticKeysCard } from "@/components/StaticKeys";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Switch } from "@/components/ui/switch";
import { RegionSelect } from "@/components/RegionSelect";
import { CommandLineInstall } from "@/components/CommandLine";
import { api, errorMessage, type AppInfo, type Settings, type UpdateInfo } from "@/lib/api";
import { copyText } from "@/lib/clipboard";
import { applyTheme, type Theme } from "@/lib/theme";
import { cn } from "@/lib/utils";

/** Fields of `cur` that differ from `base`: the edits the user makes while a save runs. */
function diff(cur: Settings, base: Settings): Partial<Settings> {
  const out: Partial<Settings> = {};
  for (const k of Object.keys(cur) as (keyof Settings)[]) {
    if (cur[k] !== base[k]) (out as Record<string, unknown>)[k] = cur[k];
  }
  return out;
}

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
  // Shows "Saved" for a moment after each successful write.
  const [justSaved, setJustSaved] = useState(false);
  useEffect(() => {
    if (!justSaved) return;
    const id = setTimeout(() => setJustSaved(false), 1500);
    return () => clearTimeout(id);
  }, [justSaved]);
  const [confirmReset, setConfirmReset] = useState(false);
  const [updateInfo, setUpdateInfo] = useState<UpdateInfo | null>(null);
  const [exporting, setExporting] = useState(false);
  const exportBundle = async () => {
    setExporting(true);
    try {
      const path = await api.ExportSupportBundle();
      toast.success("Support bundle saved", { description: path });
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setExporting(false);
    }
  };
  const [checking, setChecking] = useState(false);
  const [installing, setInstalling] = useState(false);

  const checkUpdates = async () => {
    setChecking(true);
    try {
      const update = await api.CheckForUpdates();
      setUpdateInfo(update);
      if (!update.enabled) toast.info("Updates are disabled in development builds");
      else if (update.available) toast.success(`rolle ${update.version} is available`);
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
      const msg = errorMessage(e);
      if (msg !== "cancelled") toast.error(msg);
    } finally {
      setInstalling(false);
    }
  };

  // Settings and app info load on mount and again on every open, so the first
  // open already has content and keeps its size.
  useEffect(() => {
    if (!open) setConfirmReset(false);
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
    const prev = settings;
    const next = { ...settings, ...patch };
    setSettings(next);
    if (patch.theme) applyTheme(patch.theme as Theme);
    setSaving(true);
    try {
      const saved = await api.UpdateSettings(next);
      // Merge: a toggle that raced this call keeps its own value.
      setSettings((cur) => ({ ...saved, ...(cur ? diff(cur, next) : {}) }));
      setJustSaved(true);
    } catch (e) {
      // The backend kept the old settings; show them again.
      setSettings(prev);
      if (patch.theme) applyTheme(prev.theme as Theme);
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
            Settings{" "}
            {saving ? (
              <Loader2 className="size-3.5 animate-spin text-muted-foreground" />
            ) : justSaved ? (
              <span className="flex items-center gap-1 text-xs font-normal text-emerald-600 dark:text-emerald-400">
                <Check className="size-3.5" /> Saved
              </span>
            ) : null}
          </DialogTitle>
          <DialogDescription>Preferences are saved as you change them.</DialogDescription>
        </DialogHeader>
        {!settings && <div className="h-[30.5rem]" aria-busy="true" />}
        {settings && (
          <Tabs defaultValue={new URLSearchParams(location.search).get("tab") ?? "general"} className="w-full">
            <TabsList className="grid w-full grid-cols-5">
              <TabsTrigger value="general">General</TabsTrigger>
              <TabsTrigger value="aws">AWS</TabsTrigger>
              <TabsTrigger value="appearance">Appearance</TabsTrigger>
              <TabsTrigger value="about">About</TabsTrigger>
              <TabsTrigger value="advanced">Advanced</TabsTrigger>
            </TabsList>

            <TabsContent value="general" className="mt-4 min-h-[27rem] space-y-4">
              <Row label="Terminal app" hint="Used by Open terminal on a session.">
                <Select value={settings.terminal || "auto"} onValueChange={(v) => update({ terminal: v })}>
                  <SelectTrigger className="w-64" aria-label="Terminal app">
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
                <Switch
                  aria-label="Keep running in the tray"
                  checked={settings.hideOnClose}
                  onCheckedChange={(v) => update({ hideOnClose: v })}
                />
              </Row>
              <Row
                label="Expiry notifications"
                hint="A system notification before a session expires, when it does, and before an Identity Center sign-in with active sessions runs out."
              >
                <Switch
                  aria-label="Expiry notifications"
                  checked={!settings.notifyOff}
                  onCheckedChange={(v) => update({ notifyOff: !v })}
                />
              </Row>
              {!settings.notifyOff && (
                <Row label="Warn before a session expires" hint="The tray flags the session for the same time.">
                  <Select
                    value={String(settings.notifyLeadMinutes || 2)}
                    onValueChange={(v) => update({ notifyLeadMinutes: Number(v) })}
                  >
                    <SelectTrigger className="w-64" aria-label="Warn before a session expires">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {[2, 5, 10, 15].map((m) => (
                        <SelectItem key={m} value={String(m)}>
                          {m} minutes
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </Row>
              )}
              <Row
                label="Automatic updates"
                hint="Check for new releases every few hours. Takes effect on next launch."
              >
                <Switch
                  aria-label="Automatic updates"
                  checked={!settings.autoUpdateOff}
                  onCheckedChange={(v) => update({ autoUpdateOff: !v })}
                />
              </Row>
              <Row label="Command line" hint="The rolle command ships inside the app. Link it into /usr/local/bin.">
                <CommandLineInstall compact />
              </Row>
            </TabsContent>

            <TabsContent value="aws" className="mt-4 min-h-[27rem] space-y-4">
              <Row label="Default region" hint="Pre-filled for new sessions.">
                <RegionSelect
                  value={settings.defaultRegion}
                  onChange={(v) => update({ defaultRegion: v })}
                  className="w-64 justify-between font-normal"
                />
              </Row>
              <Row
                label="Assume role duration"
                hint="For chained assume-role sessions. Identity Center roles use their permission set's session duration."
              >
                <Select
                  value={String(settings.assumeRoleMinutes)}
                  onValueChange={(v) => update({ assumeRoleMinutes: Number(v) })}
                >
                  <SelectTrigger className="w-64" aria-label="Assume role duration">
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
              <StaticKeysCard />
            </TabsContent>

            <TabsContent value="appearance" className="mt-4 min-h-[27rem] space-y-4">
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

            <TabsContent value="about" className="mt-4 min-h-[27rem] space-y-5 text-xs">
              <div className="flex items-center justify-between rounded-lg border bg-card px-3 py-2">
                <div>
                  <p className="text-sm font-medium text-foreground">rolle {info?.version ?? "…"}</p>
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
              <Row label="Update channel" hint="Beta installs pre-releases as they ship. Applies at the next check.">
                <Select
                  value={settings.updateChannel || "stable"}
                  onValueChange={(v) => update({ updateChannel: v === "beta" ? "beta" : "" })}
                >
                  <SelectTrigger className="w-40" aria-label="Update channel">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="stable">Stable</SelectItem>
                    <SelectItem value="beta">Beta</SelectItem>
                  </SelectContent>
                </Select>
              </Row>
              <div className="space-y-2">
                <p className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground">Files</p>
                <PathRow label="Workspace" value={info?.workspacePath ?? "…"} copy />
                <PathRow label="Credential cache" value={info?.cacheDir ?? "…"} copy />
                <PathRow label="AWS config" value={info?.awsConfigPath ?? "…"} copy />
                <p className="pt-1 text-[11px] text-muted-foreground">
                  Secrets live in the OS keychain. Short-lived credentials are cached with owner-only permissions and
                  expire on their own.
                </p>
              </div>
              <Row
                label="Help"
                hint="Report a problem opens the GitHub form with your version and platform filled in. The support bundle is a zip of settings, a redacted workspace, and recent diagnostics; attach it to the report."
              >
                <div className="flex shrink-0 gap-2">
                  <Button size="sm" variant="secondary" className="gap-1.5" onClick={exportBundle} disabled={exporting}>
                    {exporting ? <Loader2 className="size-3.5 animate-spin" /> : <FileArchive className="size-3.5" />}{" "}
                    Support bundle
                  </Button>
                  <Button
                    size="sm"
                    variant="secondary"
                    className="gap-1.5"
                    onClick={() => void api.SupportURL().then((u) => api.OpenURL(u))}
                  >
                    <Bug className="size-3.5" /> Report a problem
                  </Button>
                </div>
              </Row>
            </TabsContent>

            <TabsContent value="advanced" className="mt-4 min-h-[27rem] space-y-3">
              <p className="text-xs font-medium text-muted-foreground">Network</p>
              <Row label="HTTPS proxy" hint="Empty follows HTTPS_PROXY. Example: http://proxy.corp:3128">
                <Input
                  key={`proxy-${settings.proxyUrl ?? ""}`}
                  defaultValue={settings.proxyUrl ?? ""}
                  placeholder="http://host:port"
                  className="w-64 font-mono text-xs"
                  onBlur={(e) =>
                    e.target.value.trim() !== (settings.proxyUrl ?? "") && update({ proxyUrl: e.target.value.trim() })
                  }
                />
              </Row>
              <Row label="Extra CA bundle" hint="PEM file added to the OS trust store, for TLS inspection roots.">
                <Input
                  key={`ca-${settings.caBundle ?? ""}`}
                  defaultValue={settings.caBundle ?? ""}
                  placeholder="/path/to/corp-root.pem"
                  className="w-64 font-mono text-xs"
                  onBlur={(e) =>
                    e.target.value.trim() !== (settings.caBundle ?? "") && update({ caBundle: e.target.value.trim() })
                  }
                />
              </Row>
              <p className="pt-2 text-xs font-medium text-muted-foreground">Diagnostics</p>
              <Row label="Verbose logging" hint="Same as ROLLE_DEBUG=1. Prints diagnostics to the app log.">
                <Switch
                  aria-label="Verbose logging"
                  checked={settings.verboseLogging}
                  onCheckedChange={(v) => update({ verboseLogging: v })}
                />
              </Row>
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
                    onClick={run("rolle was reset", () => api.Reset())}
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
                    <Trash2 className="size-3.5" /> Reset rolle…
                  </Button>
                )}
              </div>
              <p className="text-[11px] text-muted-foreground">
                Reset removes every session, integration, keychain secret, cached credential, and rolle-owned AWS
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
