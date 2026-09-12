import { useEffect, useMemo, useRef, useState } from "react";
import {
  ArrowUpCircle,
  Cloud,
  Import,
  Keyboard,
  KeyRound,
  LogIn,
  LogOut,
  MoreHorizontal,
  Pencil,
  Plus,
  RefreshCw,
  Search,
  Settings as SettingsIcon,
  ShieldCheck,
  Star,
  Trash2,
  UserCog,
  Waypoints,
  Eye,
  EyeOff,
  Loader2,
} from "lucide-react";
import { motion } from "motion/react";
import { Events } from "@wailsio/runtime";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { ContextMenu, ContextMenuContent, ContextMenuTrigger } from "@/components/ui/context-menu";
import { ActionItems, type Action } from "@/components/ActionMenu";
import { GopherLockup, GopherMark } from "@/components/Brand";
import { SessionTable, useColumnWidths } from "./SessionTable";
import { DevTools } from "@/components/DevTools";
import { SettingsDialog } from "@/components/dialogs/SettingsDialog";
import { RenameDialog } from "@/components/dialogs/RenameDialog";
import { ImportDialog } from "@/components/dialogs/ImportDialog";
import { Key, ShortcutsDialog, combo } from "@/components/dialogs/ShortcutsDialog";
import { useModifierHeld } from "@/lib/modifier";
import {
  AddSSODialog,
  AddAssumeRoleDialog,
  AddIAMUserDialog,
  AddAzureDialog,
  AddGCPDialog,
  AddGCPImpersonationDialog,
  LoginDialog,
} from "@/components/dialogs/Dialogs";
import {
  api,
  errorMessage,
  inWails,
  Cloud as CloudKind,
  Status,
  type Integration,
  type Workspace,
  OPEN_SETTINGS,
  UPDATE_AVAILABLE,
  type UpdateInfo,
  START_NEEDS_LOGIN,
} from "@/lib/api";
import { isLoggedIn } from "@/lib/format";
import { SESSION_DRAG, TAG_DRAG } from "@/lib/drag";
import { TagGlyph } from "@/lib/tags";
import { TagDialog, type TagTarget } from "@/components/dialogs/TagDialog";
import { cn } from "@/lib/utils";

type Dialog =
  | null
  | { kind: "sso" }
  | { kind: "assume" }
  | { kind: "iam" }
  | { kind: "azure" }
  | { kind: "gcp" }
  | { kind: "gcp-impersonate" }
  | { kind: "login"; integration: Integration }
  | { kind: "settings" }
  | { kind: "rename"; integration: Integration }
  | { kind: "tag"; target: TagTarget }
  | { kind: "import" }
  | { kind: "shortcuts" };

const CLOUD_SECTIONS: { cloud: string; title: string; addKind: Dialog }[] = [
  { cloud: CloudKind.CloudAWS, title: "AWS Identity Center", addKind: { kind: "sso" } },
  { cloud: CloudKind.CloudAzure, title: "Azure tenants", addKind: { kind: "azure" } },
  { cloud: CloudKind.CloudGCP, title: "Google Cloud", addKind: { kind: "gcp" } },
];

export function Dashboard({ workspace }: { workspace: Workspace }) {
  const [query, setQuery] = useState("");
  const [widths, setWidths] = useColumnWidths();
  const [chosenFilter, setFilter] = useState<string | null>(() =>
    !inWails ? new URLSearchParams(location.search).get("filter") : null,
  );
  // ?settings=1 opens the settings dialog in the browser preview.
  // A session that waits for a sign-in. It starts when the login completes.
  const [pendingStart, setPendingStart] = useState<string | null>(null);
  const [dialog, setDialog] = useState<Dialog>(() => {
    if (inWails) return null;
    const q = new URLSearchParams(location.search);
    return q.get("settings") ? { kind: "settings" } : q.get("import") ? { kind: "import" } : null;
  });
  // The tray's Settings entry opens the dialog.
  useEffect(() => {
    if (!inWails) return;
    return Events.On(OPEN_SETTINGS, () => setDialog({ kind: "settings" }));
  }, []);
  // The tray asked to start a session that needs a sign-in first.
  useEffect(() => {
    if (!inWails) return;
    return Events.On(START_NEEDS_LOGIN, (e: { data: { sessionId: string; integrationId: string } }) => {
      const integ = workspace.integrations.find((i) => i.id === e.data.integrationId);
      if (integ) needsLogin(integ, e.data.sessionId);
    });
  });

  // A background check found a newer release. The footer offers it; the
  // updater window opens on click. The first check runs at launch, so the
  // mount asks for a result the event may have delivered before this listened.
  // ?update=1 previews it in the browser.
  const [update, setUpdate] = useState<UpdateInfo | null>(null);
  const [installing, setInstalling] = useState(false);
  useEffect(() => {
    void api.PendingUpdate().then((u) => u?.available && setUpdate(u));
    if (inWails) return Events.On(UPDATE_AVAILABLE, (e: { data: UpdateInfo }) => setUpdate(e.data));
  }, []);

  const searchRef = useRef<HTMLInputElement>(null);
  // Holding the modifier shows each shortcut as a badge next to its control.
  const held = useModifierHeld();
  const hint = (key: string) => (held ? combo(key) : undefined);
  // Keyboard shortcuts. ShortcutsDialog lists them; keep the two in step.
  useEffect(() => {
    // The same key closes the dialog it opened.
    const toggle = (kind: "settings" | "import" | "shortcuts") =>
      setDialog((d) => (d?.kind === kind ? null : { kind }));
    const onKey = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey) || e.altKey || e.shiftKey) return;
      // Digits pick the sidebar filters in the order they show, top to bottom.
      if (/^[1-9]$/.test(e.key)) {
        const key = filterKeysRef.current[Number(e.key) - 1];
        if (key !== undefined) {
          e.preventDefault();
          setFilter(key);
        }
        return;
      }
      const actions: Record<string, () => void> = {
        ",": () => toggle("settings"),
        f: () => searchRef.current?.select(),
        i: () => toggle("import"),
        "/": () => toggle("shortcuts"),
      };
      const action = actions[e.key.toLowerCase()];
      if (!action) return;
      e.preventDefault();
      action();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const active = workspace.sessions.filter((s) => s.status === Status.StatusActive).length;
  const manualCount = workspace.sessions.filter((s) => !s.integrationId).length;
  const favoriteCount = workspace.sessions.filter((s) => s.favorite).length;
  const hiddenCount = workspace.sessions.filter((s) => s.hidden).length;
  const tags = workspace.tags ?? [];
  // A tag filter is "tag:<name>"; the other filters are keywords or integration ids.
  const tagFilter = chosenFilter?.startsWith("tag:") ? chosenFilter.slice(4) : null;
  const tagCount = (tag: string) => workspace.sessions.filter((s) => !s.hidden && (s.tags ?? []).includes(tag)).length;
  // The tag a drag hovers over; the item lights up as a drop target.
  const [dropTag, setDropTag] = useState<string | null>(null);
  // Sidebar filters in display order. Cmd+1 through Cmd+9 pick them, and the
  // hold-modifier badges show each item's number.
  const filterKeys: (string | null)[] = [
    null,
    ...(active > 0 ? ["active"] : []),
    ...(favoriteCount > 0 ? ["favorites"] : []),
    ...(manualCount > 0 ? ["manual"] : []),
    ...tags.map((t) => `tag:${t.name}`),
    ...CLOUD_SECTIONS.flatMap((sec) => workspace.integrations.filter((i) => i.cloud === sec.cloud).map((i) => i.id)),
    ...(hiddenCount > 0 ? ["hidden"] : []),
  ];
  const filterKeysRef = useRef(filterKeys);
  filterKeysRef.current = filterKeys;
  const hintFor = (key: string | null) => {
    const i = filterKeys.indexOf(key);
    return i >= 0 && i < 9 ? hint(String(i + 1)) : undefined;
  };
  const visibleCount = workspace.sessions.length - hiddenCount;
  // A filter whose sidebar item is gone falls back to "All sessions".
  const filterExists =
    chosenFilter === null ||
    (chosenFilter === "manual" && manualCount > 0) ||
    (chosenFilter === "favorites" && favoriteCount > 0) ||
    (chosenFilter === "hidden" && hiddenCount > 0) ||
    (tagFilter !== null && tags.some((t) => t.name === tagFilter)) ||
    (chosenFilter === "active" && active > 0) ||
    workspace.integrations.some((i) => i.id === chosenFilter);
  const filter = filterExists ? chosenFilter : null;

  const sessions = useMemo(() => {
    const q = query.trim().toLowerCase();
    return (
      workspace.sessions
        // Hidden sessions show under their own filter, while they run, or when a
        // search names them.
        .filter((s) => (filter === "hidden" ? s.hidden : !s.hidden || s.status === Status.StatusActive || q !== ""))
        .filter((s) =>
          filter === null || filter === "hidden"
            ? true
            : filter === "manual"
              ? !s.integrationId
              : filter === "favorites"
                ? s.favorite
                : filter === "active"
                  ? s.status === Status.StatusActive
                  : tagFilter !== null
                    ? (s.tags ?? []).includes(tagFilter)
                    : s.integrationId === filter,
        )
        .filter(
          (s) =>
            !q ||
            [s.name, s.aws?.accountId, s.aws?.roleName, s.aws?.profile, s.region].some((v) =>
              (v ?? "").toLowerCase().includes(q),
            ),
        )
    );
  }, [workspace.sessions, query, filter, tagFilter]);

  async function run(label: string, fn: () => Promise<unknown>) {
    try {
      await fn();
      toast.success(label);
    } catch (e) {
      toast.error(errorMessage(e));
    }
  }

  // A Google Cloud integration has no sign-in dialog: a sync re-reads the gcloud credentials.
  function needsLogin(integ: Integration, startId?: string) {
    if (integ.cloud !== CloudKind.CloudGCP) {
      setPendingStart(startId ?? null);
      setDialog({ kind: "login", integration: integ });
      return;
    }
    void run(startId ? "Session started" : "Synced", async () => {
      await api.SyncGCP(integ.id);
      if (startId) await api.Start(startId, "");
    });
  }

  return (
    <div className="flex h-full">
      <aside className="flex w-64 shrink-0 flex-col border-r bg-sidebar text-sidebar-foreground">
        <div className="drag flex h-[4.5rem] items-center justify-end px-5 pt-2">
          <GopherLockup className="h-12" />
        </div>
        <nav className="flex-1 space-y-4 overflow-y-auto px-2 py-2">
          <div>
            <SideItem
              active={filter === null}
              onClick={() => setFilter(null)}
              label="All sessions"
              hint={hintFor(null)}
              count={visibleCount}
            />
            {active > 0 && (
              <SideItem
                active={filter === "active"}
                onClick={() => setFilter("active")}
                label="Active"
                hint={hintFor("active")}
                count={active}
                dot="ok"
              />
            )}
            {favoriteCount > 0 && (
              <SideItem
                active={filter === "favorites"}
                onClick={() => setFilter("favorites")}
                label="Favorites"
                hint={hintFor("favorites")}
                count={favoriteCount}
                icon={<Star className="size-3.5 fill-current text-brand-orange" />}
              />
            )}
            {manualCount > 0 && (
              <SideItem
                active={filter === "manual"}
                onClick={() => setFilter("manual")}
                label="Manual"
                hint={hintFor("manual")}
                count={manualCount}
              />
            )}
          </div>
          <div>
            <div className="flex items-center pr-1">
              <p className="flex-1 px-2 py-1 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
                Tags
              </p>
              <span className="flex size-6 items-center justify-center">
                <Button
                  variant="ghost"
                  size="icon-xs"
                  className="text-muted-foreground"
                  onClick={() => setDialog({ kind: "tag", target: { kind: "new" } })}
                  title="New tag"
                  aria-label="New tag"
                >
                  <Plus className="size-3.5" />
                </Button>
              </span>
            </div>
            {tags.length === 0 && <p className="px-2 py-1 text-xs text-muted-foreground/60">None yet</p>}
            {tags.map((t, index) => {
              const tag = t.name;
              const key = `tag:${tag}`;
              const actions: Action[] = [
                {
                  label: "Edit tag",
                  icon: <Pencil />,
                  onSelect: () => setDialog({ kind: "tag", target: { kind: "edit", tag: t } }),
                },
                "separator",
                {
                  label: "Remove tag",
                  icon: <Trash2 />,
                  destructive: true,
                  onSelect: () => void run(`Tag ${tag} removed`, () => api.RemoveTag(tag)),
                },
              ];
              return (
                <ContextMenu key={key}>
                  <ContextMenuTrigger asChild>
                    <SideItem
                      active={filter === key}
                      onClick={() => setFilter(key)}
                      label={tag}
                      hint={hintFor(key)}
                      count={tagCount(tag)}
                      icon={<TagGlyph tag={t} className="size-3.5" />}
                      dropping={dropTag === tag}
                      draggable
                      onDragStart={(e) => {
                        e.dataTransfer.setData(TAG_DRAG, tag);
                        e.dataTransfer.effectAllowed = "move";
                      }}
                      onDragOver={(e) => {
                        const types = Array.from(e.dataTransfer.types);
                        if (!types.includes(SESSION_DRAG) && !types.includes(TAG_DRAG)) return;
                        e.preventDefault();
                        setDropTag(tag);
                      }}
                      onDragLeave={() => setDropTag((cur) => (cur === tag ? null : cur))}
                      onDrop={(e) => {
                        e.preventDefault();
                        setDropTag(null);
                        const sessionId = e.dataTransfer.getData(SESSION_DRAG);
                        if (sessionId) {
                          void run(`Tagged ${tag}`, () => api.SetSessionTag(sessionId, tag, true));
                          return;
                        }
                        const moved = e.dataTransfer.getData(TAG_DRAG);
                        if (moved && moved !== tag) {
                          void api.MoveTag(moved, index).catch((err) => toast.error(errorMessage(err)));
                        }
                      }}
                    />
                  </ContextMenuTrigger>
                  <ContextMenuContent className="min-w-48">
                    <ActionItems actions={actions} menu="context" />
                  </ContextMenuContent>
                </ContextMenu>
              );
            })}
          </div>
          {CLOUD_SECTIONS.map((sec) => {
            const items = workspace.integrations.filter((i) => i.cloud === sec.cloud);
            return (
              <div key={sec.cloud}>
                <div className="flex items-center pr-1">
                  <p className="flex-1 px-2 py-1 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
                    {sec.title}
                  </p>
                  <span className="flex size-6 items-center justify-center">
                    <Button
                      variant="ghost"
                      size="icon-xs"
                      className="text-muted-foreground"
                      onClick={() => setDialog(sec.addKind)}
                      title="Add"
                    >
                      <Plus className="size-3.5" />
                    </Button>
                  </span>
                </div>
                {items.length === 0 && <p className="px-2 py-1 text-xs text-muted-foreground/60">None yet</p>}
                {items.map((integ) => {
                  const loggedIn = isLoggedIn(integ);
                  const sync =
                    integ.cloud === CloudKind.CloudAzure
                      ? api.SyncAzure
                      : integ.cloud === CloudKind.CloudGCP
                        ? api.SyncGCP
                        : api.SyncSSO;
                  const logout = integ.cloud === CloudKind.CloudAzure ? api.AzureLogout : api.SSOLogout;
                  const actions: Action[] = [
                    {
                      label: "Rename",
                      icon: <Pencil />,
                      onSelect: () => setDialog({ kind: "rename", integration: integ }),
                    },
                    ...(loggedIn
                      ? ([
                          {
                            label: "Sync",
                            icon: <RefreshCw />,
                            onSelect: () => void run("Synced", () => sync(integ.id)),
                          },
                          ...(integ.cloud !== CloudKind.CloudGCP
                            ? [
                                {
                                  label: "Sign out",
                                  icon: <LogOut />,
                                  onSelect: () => void run("Signed out", () => logout(integ.id)),
                                },
                              ]
                            : []),
                        ] as Action[])
                      : [
                          {
                            label: "Sign in",
                            icon: <LogIn />,
                            onSelect: () =>
                              integ.cloud === CloudKind.CloudGCP
                                ? void run("Synced", () => api.SyncGCP(integ.id))
                                : setDialog({ kind: "login", integration: integ }),
                          },
                        ]),
                    ...(integ.cloud === CloudKind.CloudGCP
                      ? [
                          {
                            label: "Impersonate service account",
                            icon: <UserCog />,
                            onSelect: () => setDialog({ kind: "gcp-impersonate" }),
                          },
                        ]
                      : []),
                    "separator",
                    {
                      label: "Remove",
                      icon: <Trash2 />,
                      destructive: true,
                      onSelect: () => void run("Removed", () => api.RemoveIntegration(integ.id)),
                    },
                  ];
                  return (
                    <ContextMenu key={integ.id}>
                      <ContextMenuTrigger asChild>
                        <SideItem
                          active={filter === integ.id}
                          onClick={() => setFilter(integ.id)}
                          label={integ.alias}
                          hint={hintFor(integ.id)}
                          dot={loggedIn ? "ok" : "off"}
                          trailing={
                            <DropdownMenu>
                              <DropdownMenuTrigger asChild>
                                <Button
                                  variant="ghost"
                                  size="icon-xs"
                                  className="text-muted-foreground opacity-0 group-hover:opacity-100 data-[state=open]:opacity-100"
                                  aria-label={`${integ.alias} options`}
                                >
                                  <MoreHorizontal className="size-3.5" />
                                </Button>
                              </DropdownMenuTrigger>
                              <DropdownMenuContent align="start" side="right" className="min-w-56">
                                <ActionItems actions={actions} menu="dropdown" />
                              </DropdownMenuContent>
                            </DropdownMenu>
                          }
                        />
                      </ContextMenuTrigger>
                      <ContextMenuContent className="min-w-56">
                        <ActionItems actions={actions} menu="context" />
                      </ContextMenuContent>
                    </ContextMenu>
                  );
                })}
              </div>
            );
          })}
          {/* Hidden sits last: out of the way, like its sessions. */}
          {hiddenCount > 0 && (
            <div>
              <ContextMenu>
                <ContextMenuTrigger asChild>
                  <div>
                    <SideItem
                      active={filter === "hidden"}
                      onClick={() => setFilter("hidden")}
                      label="Hidden"
                      hint={hintFor("hidden")}
                      count={hiddenCount}
                      icon={<EyeOff className="size-3.5" />}
                    />
                  </div>
                </ContextMenuTrigger>
                <ContextMenuContent className="min-w-40">
                  <ActionItems
                    menu="context"
                    actions={[
                      {
                        label: "Unhide all",
                        icon: <Eye />,
                        onSelect: () => void run("Every session is visible again", () => api.UnhideAll()),
                      },
                    ]}
                  />
                </ContextMenuContent>
              </ContextMenu>
            </div>
          )}
        </nav>
        <div className="flex items-center justify-between border-t p-3 text-xs text-muted-foreground">
          <div className="flex shrink-0 items-center gap-2 whitespace-nowrap">
            <span
              className={cn(
                "relative inline-block size-2 rounded-full",
                active > 0 ? "bg-emerald-400 text-emerald-400 pulse-ring" : "bg-muted-foreground/40",
              )}
            />
            {active} active
          </div>
          <div className="flex items-center gap-1">
            {update?.available && (
              <Button
                variant="ghost"
                size="sm"
                className="h-6 gap-1 px-2 text-xs text-brand-blue hover:text-brand-blue"
                aria-label={`Update to ${update.version}`}
                disabled={installing}
                onClick={() => {
                  // On a standard macOS account the download runs here and an
                  // administrator prompt follows; the button waits meanwhile.
                  setInstalling(true);
                  api
                    .InstallUpdate()
                    .catch((e) => {
                      const msg = errorMessage(e);
                      if (msg !== "cancelled") toast.error(msg);
                    })
                    .finally(() => setInstalling(false));
                }}
              >
                {installing ? <Loader2 className="size-3.5 animate-spin" /> : <ArrowUpCircle className="size-3.5" />}{" "}
                Update
              </Button>
            )}
            <DevTools />
            <span className="relative">
              {held && <Key className="absolute -top-7 right-0 h-5 text-[10px]">{combo("/")}</Key>}
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label="Keyboard shortcuts"
                onClick={() => setDialog({ kind: "shortcuts" })}
              >
                <Keyboard className="size-4" />
              </Button>
            </span>
            <span className="relative">
              {held && <Key className="absolute -top-7 right-0 h-5 text-[10px]">{combo(",")}</Key>}
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label="Settings"
                onClick={() => setDialog({ kind: "settings" })}
              >
                <SettingsIcon className="size-4" />
              </Button>
            </span>
          </div>
        </div>
      </aside>

      <section className="flex min-w-0 flex-1 flex-col">
        <header className="drag flex h-14 items-center gap-3 border-b px-5 pt-2">
          <div className="no-drag relative max-w-sm flex-1">
            <Search className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              ref={searchRef}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={(e) => e.key === "Escape" && setQuery("")}
              placeholder="Search sessions"
              className="h-8 pl-8"
            />
            {held && <Key className="absolute right-2 top-1/2 h-5 -translate-y-1/2 text-[10px]">{combo("F")}</Key>}
          </div>
          <div className="flex-1" />
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button size="sm" className="no-drag gap-1.5">
                <Plus className="size-4" /> Add
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="min-w-72">
              <DropdownMenuItem onClick={() => setDialog({ kind: "import" })}>
                <Import /> Import from this machine
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={() => setDialog({ kind: "sso" })}>
                <ShieldCheck /> AWS Identity Center portal
              </DropdownMenuItem>
              <DropdownMenuItem
                onClick={() => setDialog({ kind: "assume" })}
                disabled={!workspace.sessions.some((s) => s.aws)}
              >
                <Waypoints /> AWS assume role
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => setDialog({ kind: "iam" })}>
                <KeyRound /> AWS IAM user access key
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={() => setDialog({ kind: "azure" })}>
                <Cloud /> Azure tenant
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={() => setDialog({ kind: "gcp" })}>
                <Cloud /> Google Cloud account
              </DropdownMenuItem>
              <DropdownMenuItem
                onClick={() => setDialog({ kind: "gcp-impersonate" })}
                disabled={!workspace.integrations.some((i) => i.gcp)}
              >
                <UserCog /> GCP service account impersonation
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </header>

        <motion.div
          key={filter ?? "all"}
          initial={{ opacity: 0, y: 4 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.16, ease: "easeOut" }}
          className="flex-1 overflow-y-auto p-4"
        >
          {(() => {
            const integ = workspace.integrations.find((i) => i.id === filter);
            if (!integ || isLoggedIn(integ) || sessions.length === 0) return null;
            return (
              <p className="mb-3 px-1 text-xs text-muted-foreground">
                <span className="font-medium text-foreground">{integ.alias}</span> is signed out. Start a role to sign
                in.
              </p>
            );
          })()}
          {sessions.length === 0 ? (
            <Empty
              hasAny={workspace.sessions.length > 0}
              integration={workspace.integrations.find((i) => i.id === filter) ?? null}
              onImport={() => setDialog({ kind: "import" })}
              onConnect={() => setDialog({ kind: "sso" })}
              onSignIn={(integ) =>
                integ.cloud === CloudKind.CloudGCP
                  ? run("Synced", () => api.SyncGCP(integ.id))
                  : setDialog({ kind: "login", integration: integ })
              }
              onSync={(integ) =>
                run("Synced", () =>
                  integ.cloud === CloudKind.CloudAzure
                    ? api.SyncAzure(integ.id)
                    : integ.cloud === CloudKind.CloudGCP
                      ? api.SyncGCP(integ.id)
                      : api.SyncSSO(integ.id),
                )
              }
            />
          ) : (
            <div className="space-y-6">
              <section>
                <SessionTable
                  sessions={sessions}
                  workspace={workspace}
                  searching={query.trim() !== ""}
                  widths={widths}
                  onWidths={setWidths}
                  onNeedsLogin={needsLogin}
                />
              </section>
            </div>
          )}
        </motion.div>
      </section>

      <AddSSODialog
        open={dialog?.kind === "sso"}
        onClose={() => setDialog(null)}
        onLogin={(integ) => setDialog({ kind: "login", integration: integ })}
      />
      <AddAssumeRoleDialog open={dialog?.kind === "assume"} onClose={() => setDialog(null)} workspace={workspace} />
      <AddIAMUserDialog
        open={dialog?.kind === "iam"}
        onClose={() => setDialog(null)}
        defaultRegion={workspace.settings?.defaultRegion ?? "us-east-1"}
      />
      <AddAzureDialog
        open={dialog?.kind === "azure"}
        onClose={() => setDialog(null)}
        onLogin={(integ) => setDialog({ kind: "login", integration: integ })}
      />
      <AddGCPDialog open={dialog?.kind === "gcp"} onClose={() => setDialog(null)} />
      <AddGCPImpersonationDialog
        open={dialog?.kind === "gcp-impersonate"}
        onClose={() => setDialog(null)}
        workspace={workspace}
      />
      <LoginDialog
        integration={dialog?.kind === "login" ? dialog.integration : null}
        onClose={() => {
          setDialog(null);
          setPendingStart(null);
        }}
        onDone={(integ) => {
          setFilter(integ.id);
          if (pendingStart) {
            const id = pendingStart;
            setPendingStart(null);
            api
              .Start(id, "")
              .then(() => toast.success("Session started"))
              .catch((e) => toast.error(errorMessage(e)));
          }
        }}
      />
      <SettingsDialog open={dialog?.kind === "settings"} onClose={() => setDialog(null)} />
      <ShortcutsDialog open={dialog?.kind === "shortcuts"} onClose={() => setDialog(null)} />
      <ImportDialog
        open={dialog?.kind === "import"}
        onClose={() => setDialog(null)}
        workspace={workspace}
        onLogin={(integ) => setDialog({ kind: "login", integration: integ })}
      />
      <RenameDialog
        target={
          dialog?.kind === "rename"
            ? {
                kind: "integration",
                id: dialog.integration.id,
                name: dialog.integration.alias,
                save: (n) => api.RenameIntegration(dialog.integration.id, n),
              }
            : null
        }
        onClose={() => setDialog(null)}
      />
      <TagDialog
        target={dialog?.kind === "tag" ? dialog.target : null}
        onClose={() => setDialog(null)}
        onRenamed={(from, to) => setFilter((f) => (f === `tag:${from}` ? `tag:${to}` : f))}
      />
    </div>
  );
}

function SideItem({
  active,
  onClick,
  label,
  count,
  dot,
  icon,
  trailing,
  hint,
  dropping,
  ...rest
}: {
  active: boolean;
  onClick: () => void;
  label: string;
  count?: number;
  dot?: "ok" | "off";
  icon?: React.ReactNode;
  trailing?: React.ReactNode;
  /** Shortcut badge shown while the modifier is held. */
  hint?: string;
  /** A drag hovers over this item. */
  dropping?: boolean;
} & React.ComponentProps<"div">) {
  return (
    <div
      {...rest}
      className={cn(
        "group relative flex items-center rounded-md pr-1 transition-colors",
        active
          ? "bg-sidebar-accent text-sidebar-accent-foreground"
          : "text-muted-foreground hover:bg-sidebar-accent/60 hover:text-foreground",
        dropping && "bg-sidebar-accent/60 text-foreground ring-1 ring-ring",
      )}
    >
      <button
        type="button"
        onClick={onClick}
        // The dot alone is colour only; the title names the state for hover and assistive tech.
        title={dot ? (dot === "ok" ? "Signed in" : "Signed out") : undefined}
        className="flex min-w-0 flex-1 items-center gap-2 px-2 py-1.5 text-left text-sm"
      >
        {icon}
        {dot && (
          <span
            aria-hidden
            className={cn("size-1.5 shrink-0 rounded-full", dot === "ok" ? "bg-emerald-400" : "bg-muted-foreground/40")}
          />
        )}
        <span className="truncate">{label}</span>
        {count !== undefined && (
          <span
            className={cn(
              "rounded-full px-1.5 py-px text-[10px] tabular-nums",
              active ? "bg-background/70 text-foreground/70" : "bg-muted text-muted-foreground",
            )}
          >
            {count}
          </span>
        )}
        <span className="flex-1" />
      </button>
      {/* Fixed slot keeps counts aligned whether or not a row has a control. */}
      <span className="flex size-6 shrink-0 items-center justify-center">{trailing}</span>
      {hint && <Key className="absolute right-1.5 top-1/2 h-5 -translate-y-1/2 text-[10px]">{hint}</Key>}
    </div>
  );
}

function Empty({
  hasAny,
  integration,
  onImport,
  onConnect,
  onSignIn,
  onSync,
}: {
  hasAny: boolean;
  integration: Integration | null;
  onImport: () => void;
  onConnect: () => void;
  onSignIn: (i: Integration) => void;
  onSync: (i: Integration) => void;
}) {
  // A selected integration gets its own guidance: sign in, or sync when signed in but empty.
  if (integration) {
    const signedIn = isLoggedIn(integration);
    return (
      <div className="flex h-full flex-col items-center justify-center text-center">
        <GopherMark className="size-24" />
        <h3 className="mt-5 text-lg font-medium">
          {signedIn ? `No sessions in ${integration.alias}` : `${integration.alias} is signed out`}
        </h3>
        <p className="mt-1 max-w-sm text-sm text-muted-foreground">
          {signedIn ? "Sync to discover what this account can reach." : "Sign in to discover its sessions."}
        </p>
        <div className="mt-5 flex gap-2">
          {signedIn ? (
            <Button className="gap-1.5" onClick={() => onSync(integration)}>
              <RefreshCw className="size-4" /> Sync {integration.alias}
            </Button>
          ) : (
            <Button className="gap-1.5" onClick={() => onSignIn(integration)}>
              <LogIn className="size-4" /> Sign in to {integration.alias}
            </Button>
          )}
        </div>
      </div>
    );
  }
  return (
    <div className="flex h-full flex-col items-center justify-center text-center">
      <GopherMark className="size-24" />
      <h3 className="mt-5 text-lg font-medium">{hasAny ? "Nothing matches" : "No sessions yet"}</h3>
      <p className="mt-1 max-w-sm text-sm text-muted-foreground">
        {hasAny
          ? "Try a different search or filter."
          : "Bring in what your cloud CLIs already know, or connect a cloud from the Add menu."}
      </p>
      {!hasAny && (
        <div className="mt-5 flex gap-2">
          <Button className="gap-1.5" onClick={onImport}>
            <Import className="size-4" /> Import from this machine
          </Button>
          <Button variant="secondary" className="gap-1.5" onClick={onConnect}>
            <ShieldCheck className="size-4" /> Connect AWS Identity Center
          </Button>
        </div>
      )}
    </div>
  );
}
