import { useState } from "react";
import { motion } from "motion/react";
import { Clipboard, ExternalLink, Loader2, MoreHorizontal, Play, Square, Terminal, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { Clipboard as WailsClipboard } from "@wailsio/runtime";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { CloudGlyph } from "@/components/Brand";
import { MFADialog } from "@/components/dialogs/Dialogs";
import { api, errorMessage, Kind, Status, type Session, type Workspace } from "@/lib/api";
import { celebrate } from "@/lib/celebrate";
import { cloudOf, kindLabel, remaining, sessionSubtitle, useNow } from "@/lib/format";
import { cn } from "@/lib/utils";

export function SessionRow({ session: s, workspace }: { session: Session; workspace: Workspace }) {
  const now = useNow();
  const [busy, setBusy] = useState(false);
  const [mfaOpen, setMfaOpen] = useState(false);
  const active = s.status === Status.StatusActive;
  const needsMFA = s.kind === Kind.KindAWSIAMUser && !!s.aws?.mfaDevice;
  const source = s.aws?.sourceSessionId ? workspace.sessions.find((x) => x.id === s.aws?.sourceSessionId) : undefined;

  async function start(mfaCode = "") {
    setBusy(true);
    try {
      await api.Start(s.id, mfaCode);
      const first = !workspace.sessions.some((x) => x.status === Status.StatusActive);
      if (first) celebrate("small");
      toast.success(`${s.name} started`, { description: isAWS ? `aws --profile ${await api.ProfileName(s.id)}` : `eval "$(rolle env ${s.name})"` });
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }

  async function stop() {
    setBusy(true);
    try {
      await api.Stop(s.id);
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }

  const isAWS = cloudOf(s.kind) === "aws";

  async function copy(kind: "env" | "profile") {
    try {
      const text = kind === "profile" ? `aws --profile ${await api.ProfileName(s.id)}` : await api.EnvText(s.id);
      await WailsClipboard.SetText(text);
      toast.success(kind === "profile" ? "Profile command copied" : "Credentials copied", { description: kind === "env" ? "Paste into a shell. They expire on their own." : undefined });
    } catch (e) {
      toast.error(errorMessage(e));
    }
  }

  return (
    <motion.li
      layout
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, scale: 0.98 }}
      transition={{ type: "spring", stiffness: 320, damping: 28 }}
      className={cn("group flex items-center gap-3 rounded-lg border bg-card px-3 py-2.5 transition-colors hover:bg-accent/60", active && "border-emerald-500/40")}
    >
      <span className={cn("relative size-2 shrink-0 rounded-full", active ? "bg-emerald-400 text-emerald-400 pulse-ring" : "bg-muted-foreground/30")} />
      <CloudGlyph cloud={cloudOf(s.kind)} />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <p className="truncate text-sm font-medium">{s.name}</p>
          <Badge variant="outline" className="h-5 px-1.5 text-[10px] font-normal text-muted-foreground">{kindLabel[s.kind] ?? s.kind}</Badge>
          {s.region && <span className="text-[11px] text-muted-foreground/70">{s.region}</span>}
        </div>
        <p className="truncate font-mono text-[11px] text-muted-foreground">{sessionSubtitle(s)}{source ? ` · via ${source.name}` : ""}</p>
      </div>
      <span className={cn("flex w-28 items-center justify-end gap-1.5 font-mono text-xs tabular-nums", active ? "text-emerald-300" : "text-muted-foreground/70")}>
        {active ? <><span className="font-sans font-medium">Active</span><span>{remaining(s.expires, now)}</span></> : <span className="font-sans">Inactive</span>}
      </span>
      <div className={cn("flex items-center gap-0.5 transition-opacity", active ? "opacity-100" : "opacity-0 group-hover:opacity-100 group-focus-within:opacity-100")}>
        {active && (
          <>
            <IconBtn label="Open console" onClick={() => api.OpenConsole(s.id).catch((e) => toast.error(errorMessage(e)))}><ExternalLink /></IconBtn>
            {isAWS && <IconBtn label="Copy profile command" onClick={() => copy("profile")}><Terminal /></IconBtn>}
            <IconBtn label="Copy credentials as env" onClick={() => copy("env")}><Clipboard /></IconBtn>
          </>
        )}
        <DropdownMenu>
          <DropdownMenuTrigger asChild><Button variant="ghost" size="icon-sm"><MoreHorizontal /></Button></DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            {isAWS && <DropdownMenuItem onClick={() => copy("profile")}><Terminal /> Copy profile command</DropdownMenuItem>}
            {isAWS && <DropdownMenuSeparator />}
            <DropdownMenuItem variant="destructive" onClick={() => api.RemoveSession(s.id).catch((e) => toast.error(errorMessage(e)))}><Trash2 /> Remove</DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      <Button
        size="sm"
        variant={active ? "secondary" : "default"}
        className="w-20 gap-1.5"
        disabled={busy}
        onClick={() => (active ? stop() : needsMFA ? setMfaOpen(true) : start())}
      >
        {busy ? <Loader2 className="size-3.5 animate-spin" /> : active ? <Square className="size-3.5" /> : <Play className="size-3.5" />}
        {active ? "Stop" : "Start"}
      </Button>
      <MFADialog open={mfaOpen} onClose={() => setMfaOpen(false)} onSubmit={(code) => { setMfaOpen(false); void start(code); }} />
    </motion.li>
  );
}

function IconBtn({ label, onClick, children }: { label: string; onClick: () => void; children: React.ReactNode }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button variant="ghost" size="icon-sm" onClick={onClick} aria-label={label}>{children}</Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}
