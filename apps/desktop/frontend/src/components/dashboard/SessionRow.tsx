import { useState } from "react";
import {
  Clipboard,
  ExternalLink,
  Globe,
  Loader2,
  MoreHorizontal,
  Pencil,
  Play,
  Square,
  SquareTerminal,
  Star,
  TriangleAlert,
  Terminal,
  Trash2,
  Eye,
  EyeOff,
  Check,
  X,
} from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { motion } from "motion/react";
import { TableCell } from "@/components/ui/table";
import { ContextMenu, ContextMenuContent, ContextMenuTrigger } from "@/components/ui/context-menu";
import { ActionItems, type Action } from "@/components/ActionMenu";
import { RegionDialog } from "@/components/dialogs/RegionDialog";
import { CloudGlyph } from "@/components/Brand";
import { MFADialog } from "@/components/dialogs/Dialogs";
import { RenameDialog, type RenameTarget } from "@/components/dialogs/RenameDialog";
import { RemoveKeysDialog, type RemoveKeysTarget } from "@/components/dialogs/RemoveKeysDialog";
import { api, errorMessage, Kind, Status, type Integration, type Session, type Workspace } from "@/lib/api";
import { celebrate } from "@/lib/celebrate";
import { copyText } from "@/lib/clipboard";
import { cloudOf, kindLabel, remaining, sessionSubtitle, useNow } from "@/lib/format";
import { cn } from "@/lib/utils";
import { SESSION_DRAG, setSessionDragImage } from "@/lib/drag";
import { TagGlyph } from "@/lib/tags";

const isAWSKind = (k: string) => cloudOf(k) === "aws";

/** The part of an "account/role" name after the account. A renamed session shows its own name. */
const roleLabel = (name: string) => name.split("/").slice(1).join("/") || name;

/** A session row. A nested row sits under its account row and shows the role name only. */
export function SessionRow({
  session: s,
  workspace,
  nested,
  onNeedsLogin,
  onTagClick,
  shadows,
}: {
  session: Session;
  workspace: Workspace;
  nested?: boolean;
  onNeedsLogin?: (integration: Integration, startSessionId?: string) => void;
  /** The name on a tag chip was clicked. */
  onTagClick?: (tag: string) => void;
  /** Profile name to the file whose static keys shadow it. */
  shadows?: Record<string, string | undefined>;
}) {
  const [busy, setBusy] = useState(false);
  const [mfaOpen, setMfaOpen] = useState(false);
  const [editing, setEditing] = useState<RenameTarget | null>(null);
  // The static keys to remove from ~/.aws/credentials, and whether to start after.
  const [removing, setRemoving] = useState<(RemoveKeysTarget & { thenStart: boolean }) | null>(null);
  const [regionOpen, setRegionOpen] = useState(false);
  const profileName = isAWSKind(s.kind) ? s.aws?.profile || "default" : "";
  // Static keys under the same profile name win over this session.
  const shadowedBy = isAWSKind(s.kind) ? shadows?.[profileName] : undefined;
  const shadowNote = shadowedBy ? `A profile with this name in ${shadowedBy} wins. Click to fix.` : "";
  const tags = workspace.tags ?? [];
  const active = s.status === Status.StatusActive;
  const needsMFA = s.kind === Kind.KindAWSIAMUser && !!s.aws?.mfaDevice;
  const source = s.aws?.sourceSessionId ? workspace.sessions.find((x) => x.id === s.aws?.sourceSessionId) : undefined;

  // shadowToast names the file and offers the fix, which starts the session after.
  function shadowToast() {
    toast.error("Local profile conflict", {
      description: `${profileName} in ${shadowedBy ?? "~/.aws/credentials"}`,
      action: { label: "Fix", onClick: () => setRemoving({ profiles: [profileName], thenStart: true }) },
    });
  }

  // start runs the session. A profile that static keys already shadow cannot
  // start, so the toast comes at once; afterFix skips that check, since the
  // shadow map refreshes with the next workspace event.
  async function start(mfaCode = "", afterFix = false) {
    if (busy) return;
    if (shadowedBy && !afterFix) {
      shadowToast();
      return;
    }
    setBusy(true);
    // Start first; the backend may renew the portal token silently. A "login
    // required" error is the one signal that the browser is needed.
    const integration = workspace.integrations.find((i) => i.id === s.integrationId);
    try {
      await api.Start(s.id, mfaCode);
      const first = !workspace.sessions.some((x) => x.status === Status.StatusActive);
      if (first) celebrate("small");
      // The role is the title; the check mark says it started. The account
      // of an Identity Center role and the AWS profile follow.
      const [account, role] =
        s.kind === Kind.KindAWSSSORole && s.name.includes("/")
          ? [s.name.split("/")[0], roleLabel(s.name)]
          : ["", s.name];
      toast.success(role, {
        description: (
          <>
            {account && <span className="block">Account: {account}</span>}
            {isAWS && <span className="block">Profile: {profileName}</span>}
          </>
        ),
        action: { label: "Copy env", onClick: () => void copy("env") },
      });
    } catch (e) {
      const msg = errorMessage(e);
      if (/login required/i.test(msg) && integration && onNeedsLogin) {
        onNeedsLogin(integration, s.id);
      } else if (/static keys/i.test(msg)) {
        shadowToast();
      } else {
        toast.error(msg);
      }
    } finally {
      setBusy(false);
    }
  }

  async function stop() {
    if (busy) return;
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
  // The provider mark already names the cloud; only less common kinds get a badge.
  const badge = s.kind === Kind.KindAWSAssumeRole || s.kind === Kind.KindAWSIAMUser ? kindLabel[s.kind] : "";

  async function copy(kind: "env" | "profile") {
    try {
      const text = kind === "profile" ? `aws --profile ${profileName}` : await api.EnvText(s.id);
      await copyText(text);
      toast.success("Copied", { description: kind === "profile" ? text : "Credentials for your shell" });
    } catch (e) {
      toast.error(errorMessage(e));
    }
  }

  // Set for an Identity Center role; its permission set can be renamed in every account.
  const roleName = s.kind === Kind.KindAWSSSORole ? s.aws?.roleName : undefined;
  // One action list feeds the row menu and the right-click menu.
  const common: Action[] = [
    {
      label: s.favorite ? "Remove from favorites" : "Add to favorites",
      icon: <Star />,
      onSelect: () => void api.SetFavorite(s.id, !s.favorite).catch((e) => toast.error(errorMessage(e))),
    },
    {
      label: s.hidden ? "Unhide" : "Hide",
      icon: s.hidden ? <Eye /> : <EyeOff />,
      onSelect: () => void api.SetHidden(s.id, !s.hidden).catch((e) => toast.error(errorMessage(e))),
    },
    ...(tags.length > 0
      ? ([
          {
            label: "Tags",
            icon: <TagGlyph tag={{ color: "", icon: "tag" }} />,
            items: tags.map((t) => {
              const on = (s.tags ?? []).includes(t.name);
              return {
                label: t.name,
                icon: on ? <Check /> : <TagGlyph tag={t} />,
                onSelect: () => void api.SetSessionTag(s.id, t.name, !on).catch((e) => toast.error(errorMessage(e))),
              };
            }),
          },
        ] as Action[])
      : []),
    {
      label: "Rename",
      icon: <Pencil />,
      onSelect: () =>
        setEditing({
          kind: "session",
          id: s.id,
          name: s.name,
          save: (n) => api.RenameSession(s.id, n),
          // An Identity Center role can take the name in every account instead.
          everywhere: roleName
            ? {
                label: "Apply to this role in every account",
                name: roleLabel(s.name),
                save: (n) => api.SetAlias("role", roleName, n),
              }
            : undefined,
        }),
    },
    ...(isAWS
      ? ([
          {
            label: "Set AWS profile name",
            icon: <Terminal />,
            onSelect: () =>
              setEditing({
                kind: "profile",
                id: s.id,
                name: s.aws?.profile ?? "",
                save: (n) => api.SetProfile(s.id, n),
              }),
          },
          { label: "Change region", icon: <Globe />, onSelect: () => setRegionOpen(true) },
          { label: "Copy profile command", icon: <Terminal />, onSelect: () => void copy("profile") },
          "separator",
        ] as Action[])
      : []),
    {
      label: "Remove",
      icon: <Trash2 />,
      destructive: true,
      onSelect: () => void api.RemoveSession(s.id).catch((e) => toast.error(errorMessage(e))),
    },
  ];
  const contextActions: Action[] = [
    {
      label: active ? "Stop" : "Start",
      icon: active ? <Square /> : <Play />,
      onSelect: () => (active ? void stop() : needsMFA ? setMfaOpen(true) : void start()),
    },
    ...(active
      ? ([
          {
            label: "Open terminal here",
            icon: <SquareTerminal />,
            onSelect: () => void api.OpenTerminal(s.id).catch((e) => toast.error(errorMessage(e))),
          },
          {
            label: "Open console",
            icon: <ExternalLink />,
            onSelect: () => void api.OpenConsole(s.id).catch((e) => toast.error(errorMessage(e))),
          },
          { label: "Copy credentials as env", icon: <Clipboard />, onSelect: () => void copy("env") },
        ] as Action[])
      : []),
    "separator",
    ...common,
  ];

  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>
        <motion.tr
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ duration: 0.15, ease: "easeOut" }}
          draggable
          // motion.tr owns onDragStart for its own gesture; the capture
          // variant is the native drag event.
          onDragStartCapture={(e) => {
            e.dataTransfer.setData(SESSION_DRAG, s.id);
            e.dataTransfer.effectAllowed = "link";
            setSessionDragImage(e, s.name);
          }}
          className={cn(
            "group border-b transition-colors hover:bg-muted/50",
            nested && "bg-muted/15",
            active && "bg-emerald-500/[0.04]",
            s.hidden && "opacity-60",
          )}
        >
          <TableCell className="pr-0">
            <div className="flex items-center gap-1.5">
              <button
                type="button"
                aria-label={active ? "Stop" : "Start"}
                disabled={busy}
                onClick={() => (active ? stop() : needsMFA ? setMfaOpen(true) : start())}
                className={cn(
                  "relative inline-flex size-7 items-center justify-center rounded-full border transition-colors disabled:opacity-60",
                  active
                    ? "border-emerald-500/40 bg-emerald-500/15 text-emerald-700 hover:bg-emerald-500/25 dark:text-emerald-300"
                    : "border-border text-muted-foreground hover:border-foreground/40 hover:bg-accent hover:text-foreground",
                )}
              >
                {busy ? (
                  <Loader2 className="size-3.5 animate-spin" />
                ) : active ? (
                  <Square className="size-2.5 fill-current" />
                ) : (
                  <Play className="size-3 fill-current" />
                )}
              </button>
              <button
                type="button"
                aria-label={s.favorite ? "Remove from favorites" : "Add to favorites"}
                onClick={() => api.SetFavorite(s.id, !s.favorite).catch((e) => toast.error(errorMessage(e)))}
                className={cn(
                  "rounded p-1 transition-opacity",
                  s.favorite
                    ? "text-brand-orange"
                    : "text-muted-foreground/50 opacity-0 hover:text-foreground group-hover:opacity-100 group-focus-within:opacity-100",
                )}
              >
                <Star className={cn("size-4", s.favorite && "fill-current")} />
              </button>
            </div>
          </TableCell>
          <TableCell>
            <div className={cn("flex items-center gap-2.5", nested && "pl-10")}>
              {!nested && <CloudGlyph cloud={cloudOf(s.kind)} />}
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <p className="flex items-center gap-1.5 truncate text-sm font-medium">
                    {nested ? roleLabel(s.name) : s.name}
                    {s.hidden && <EyeOff className="size-3 shrink-0 text-muted-foreground" aria-label="Hidden" />}
                  </p>
                  {badge && (
                    <Badge
                      variant="outline"
                      className="h-5 shrink-0 px-1.5 text-[10px] font-normal text-muted-foreground"
                    >
                      {badge}
                    </Badge>
                  )}
                  {(s.tags ?? []).map((name) => {
                    const t = tags.find((x) => x.name === name) ?? { name, color: "", icon: "" };
                    return (
                      <motion.span
                        key={name}
                        initial={{ scale: 0.6, opacity: 0 }}
                        animate={{ scale: 1, opacity: 1 }}
                        transition={{ type: "spring", stiffness: 500, damping: 28 }}
                        className="inline-flex"
                      >
                        <Badge
                          variant="secondary"
                          className="group/chip h-5 shrink-0 gap-1 px-1.5 text-[10px] font-normal text-muted-foreground"
                        >
                          <button
                            type="button"
                            onClick={() => onTagClick?.(name)}
                            className="inline-flex items-center gap-1 hover:text-foreground"
                          >
                            <TagGlyph tag={t} className="size-2.5" /> {name}
                          </button>
                          <button
                            type="button"
                            aria-label={`Remove tag ${name}`}
                            onClick={() =>
                              void api.SetSessionTag(s.id, name, false).catch((e) => toast.error(errorMessage(e)))
                            }
                            className="-mr-0.5 hidden rounded-full hover:text-foreground group-hover/chip:inline-flex group-focus-within/chip:inline-flex"
                          >
                            <X className="size-2.5" />
                          </button>
                        </Badge>
                      </motion.span>
                    );
                  })}
                </div>
                {!nested && (
                  <p className="truncate font-mono text-[11px] text-muted-foreground">
                    {sessionSubtitle(s)}
                    {source ? ` · via ${source.name}` : ""}
                  </p>
                )}
              </div>
            </div>
          </TableCell>
          <TableCell>
            {isAWS ? (
              <button
                type="button"
                onClick={() =>
                  setEditing({
                    kind: "profile",
                    id: s.id,
                    name: s.aws?.profile ?? "",
                    save: (n) => api.SetProfile(s.id, n),
                    check: async (n) => {
                      const path = await api.ProfileShadow(n || "default");
                      return path ? `${path} has a profile with this name.` : "";
                    },
                    onFix: (n) => {
                      setEditing(null);
                      setRemoving({ profiles: [n || "default"], thenStart: false });
                    },
                  })
                }
                className={cn(
                  "inline-flex items-center gap-1 rounded px-1.5 py-0.5 font-mono text-xs text-muted-foreground hover:bg-accent hover:text-foreground",
                  shadowedBy && "text-amber-600 dark:text-amber-400",
                )}
                title={shadowNote || "Change the AWS profile name"}
              >
                {shadowedBy && <TriangleAlert aria-label="Profile is shadowed" className="size-3" />}
                {profileName}
              </button>
            ) : (
              <span className="text-xs text-muted-foreground/50">—</span>
            )}
          </TableCell>
          <TableCell>
            {isAWS ? (
              <button
                type="button"
                title="Change the region"
                onClick={() => setRegionOpen(true)}
                className="rounded px-1.5 py-0.5 font-mono text-xs text-muted-foreground hover:bg-accent hover:text-foreground"
              >
                {s.region || "—"}
              </button>
            ) : (
              <span className="text-xs text-muted-foreground/50">—</span>
            )}
          </TableCell>
          <TableCell>
            <span
              className={cn(
                "flex items-center gap-1.5 font-mono text-xs tabular-nums",
                active ? "text-emerald-700 dark:text-emerald-300" : "text-muted-foreground/70",
              )}
            >
              {active ? (
                <>
                  <span className="font-sans font-medium">Active</span>
                  <Countdown expires={s.expires} />
                </>
              ) : (
                <span className="font-sans">Inactive</span>
              )}
            </span>
          </TableCell>
          <TableCell className="text-right">
            <div className="flex items-center justify-end gap-0.5">
              <div
                className={cn(
                  "flex items-center gap-0.5 transition-opacity",
                  active ? "opacity-100" : "opacity-0 group-hover:opacity-100 group-focus-within:opacity-100",
                )}
              >
                {active && (
                  <>
                    <IconBtn
                      label="Open terminal here"
                      onClick={() => api.OpenTerminal(s.id).catch((e) => toast.error(errorMessage(e)))}
                    >
                      <SquareTerminal />
                    </IconBtn>
                    <IconBtn
                      label="Open console"
                      onClick={() => api.OpenConsole(s.id).catch((e) => toast.error(errorMessage(e)))}
                    >
                      <ExternalLink />
                    </IconBtn>
                    {isAWS && (
                      <IconBtn label="Copy profile command" onClick={() => copy("profile")}>
                        <Terminal />
                      </IconBtn>
                    )}
                    <IconBtn label="Copy credentials as env" onClick={() => copy("env")}>
                      <Clipboard />
                    </IconBtn>
                  </>
                )}
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button variant="ghost" size="icon-sm" aria-label="More actions">
                      <MoreHorizontal />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" className="min-w-56">
                    <ActionItems actions={common} menu="dropdown" />
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            </div>
            <MFADialog
              open={mfaOpen}
              onClose={() => setMfaOpen(false)}
              onSubmit={(code) => {
                setMfaOpen(false);
                void start(code);
              }}
            />
            <RenameDialog target={editing} onClose={() => setEditing(null)} />
            <RemoveKeysDialog
              target={removing}
              onClose={() => setRemoving(null)}
              onDone={() => removing?.thenStart && void start("", true)}
            />
            <RegionDialog session={regionOpen ? s : null} onClose={() => setRegionOpen(false)} />
          </TableCell>
        </motion.tr>
      </ContextMenuTrigger>
      <ContextMenuContent className="min-w-56">
        <ActionItems actions={contextActions} menu="context" />
      </ContextMenuContent>
    </ContextMenu>
  );
}

function IconBtn({ label, onClick, children }: { label: string; onClick: () => void; children: React.ReactNode }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button variant="ghost" size="icon-sm" onClick={onClick} aria-label={label}>
          {children}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

/** Time left on an active session. Ticks once a second on its own. */
function Countdown({ expires }: { expires: string | null | undefined }) {
  const now = useNow();
  return <span>{remaining(expires, now)}</span>;
}
