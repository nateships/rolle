import { Fragment, useEffect, useMemo, useRef, useState } from "react";
import {
  ArrowUpCircle,
  Cloud,
  Import,
  Keyboard,
  KeyRound,
  LayoutList,
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
  UserRound,
  Waypoints,
  Eye,
  EyeOff,
  Loader2,
} from "lucide-react";
import { AnimatePresence, motion } from "motion/react";
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
import { CloudMark, GopherLockup, GopherMark } from "@/components/Brand";
import { HelpMenu } from "@/components/dashboard/HelpMenu";
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
  Kind,
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

const SIDEBAR_KEY = "rolle.sidebar";
const SIDEBAR_DEFAULT = 256;
// The lockup is 48px tall: a 56px mark, a 64px wordmark, and 20px of right
// padding. The macOS traffic lights end near x=70. This floor keeps the
// lockup clear of them with a small gap.
const SIDEBAR_MIN = 228;
const SIDEBAR_MAX = 420;

/** Sidebar width in pixels, kept between launches. */
function useSidebarWidth() {
  const [width, setWidth] = useState(() => {
    try {
      const n = Number(localStorage.getItem(SIDEBAR_KEY));
      return n >= SIDEBAR_MIN && n <= SIDEBAR_MAX ? n : SIDEBAR_DEFAULT;
    } catch {
      return SIDEBAR_DEFAULT;
    }
  });
  useEffect(() => {
    try {
      localStorage.setItem(SIDEBAR_KEY, String(width));
    } catch {
      /* storage is optional */
    }
  }, [width]);
  return [width, setWidth] as const;
}

// key matches core.SidebarSections; Settings → Appearance hides a section by it.
const CLOUD_SECTIONS: { key: string; cloud: string; mark: "aws" | "azure" | "gcp"; title: string; addKind: Dialog }[] =
  [
    { key: "aws-sso", cloud: CloudKind.CloudAWS, mark: "aws", title: "AWS Identity Center", addKind: { kind: "sso" } },
    { key: "azure", cloud: CloudKind.CloudAzure, mark: "azure", title: "Azure tenants", addKind: { kind: "azure" } },
    { key: "gcp", cloud: CloudKind.CloudGCP, mark: "gcp", title: "Google Cloud", addKind: { kind: "gcp" } },
  ];

export function Dashboard({ workspace }: { workspace: Workspace }) {
  const [query, setQuery] = useState("");
  const [widths, setWidths] = useColumnWidths();
  const [sidebarWidth, setSidebarWidth] = useSidebarWidth();
  const [chosenFilter, setFilter] = useState<string | null>(() =>
    !inWails ? new URLSearchParams(location.search).get("filter") : null,
  );
  // ?settings=1 opens the settings dialog in the browser preview.
  // A session that waits for a sign-in. It starts when the login completes.
  // The session to start once a sign-in completes. A ref, so the login
  // dialog's completion handler reads the latest value, not the one from
  // the render that opened it.
  const pendingStart = useRef<string | null>(null);
  const [dialog, setDialog] = useState<Dialog>(() => {
    if (inWails) return null;
    const q = new URLSearchParams(location.search);
    return q.get("settings") ? { kind: "settings" } : q.get("import") ? { kind: "import" } : null;
  });
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
      pendingStart.current = startId ?? null;
      setDialog({ kind: "login", integration: integ });
      return;
    }
    void run(startId ? "Started" : "Synced", async () => {
      await api.SyncGCP(integ.id);
      if (startId) await api.Start(startId, "");
    });
  }

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

  const active = workspace.sessions.filter((s) => s.status === Status.StatusActive).length;
  // Hidden sessions leave every list but Hidden, so the counts skip them too.
  // IAM users and assumed roles have no integration; they get their own AWS section.
  const iamCount = workspace.sessions.filter((s) => !s.hidden && s.kind === Kind.KindAWSIAMUser).length;
  const roleCount = workspace.sessions.filter((s) => !s.hidden && s.kind === Kind.KindAWSAssumeRole).length;
  // Sections the user hid in Settings → Appearance. Their filters leave the shortcut order too.
  const hiddenSections = useMemo(() => workspace.settings?.hiddenSections ?? [], [workspace.settings?.hiddenSections]);
  const showSection = (key: string) => !hiddenSections.includes(key);
  const iamKeys = useMemo(
    () =>
      hiddenSections.includes("aws-iam")
        ? []
        : [...(iamCount > 0 ? ["iam-users"] : []), ...(roleCount > 0 ? ["assumed-roles"] : [])],
    [iamCount, roleCount, hiddenSections],
  );
  const favoriteCount = workspace.sessions.filter((s) => !s.hidden && s.favorite).length;
  const hiddenCount = workspace.sessions.filter((s) => s.hidden).length;
  const tags = useMemo(() => workspace.tags ?? [], [workspace.tags]);
  // A tag filter is "tag:<name>"; the other filters are keywords or integration ids.
  const tagFilter = chosenFilter?.startsWith("tag:") ? chosenFilter.slice(4) : null;
  const tagCount = (tag: string) => workspace.sessions.filter((s) => !s.hidden && (s.tags ?? []).includes(tag)).length;
  // The tag a drag hovers over; the item lights up as a drop target.
  const [dropTag, setDropTag] = useState<string | null>(null);
  // The tag that just took a drop; it pops for a moment.
  const [poppedTag, setPoppedTag] = useState<string | null>(null);
  // Where a dragged tag would land: a line above or below the hovered tag.
  const [dropLine, setDropLine] = useState<{ tag: string; side: "before" | "after" } | null>(null);
  // The tag being dragged; it cannot land on itself.
  const draggingTag = useRef<string | null>(null);
  // The pointer's half of the row picks the side of the line.
  const lineSide = (e: React.DragEvent): "before" | "after" => {
    const box = e.currentTarget.getBoundingClientRect();
    return e.clientY < box.top + box.height / 2 ? "before" : "after";
  };
  const pop = (key: string) => {
    setPoppedTag(key);
    window.setTimeout(() => setPoppedTag((cur) => (cur === key ? null : cur)), 600);
  };
  // Favorites and Hidden take a dropped session like a tag does.
  const sessionDrop = (key: string, done: string, act: (id: string) => Promise<unknown>) => ({
    dropping: dropTag === key,
    popped: poppedTag === key,
    onDragOver: (e: React.DragEvent) => {
      if (!Array.from(e.dataTransfer.types).includes(SESSION_DRAG)) return;
      e.preventDefault();
      setDropTag(key);
    },
    onDragLeave: () => setDropTag((cur) => (cur === key ? null : cur)),
    onDrop: (e: React.DragEvent) => {
      e.preventDefault();
      setDropTag(null);
      const id = e.dataTransfer.getData(SESSION_DRAG);
      if (!id) return;
      pop(key);
      void run(done, () => act(id));
    },
  });
  // Sidebar filters in display order. Cmd+1 through Cmd+9 pick them, and the
  // hold-modifier badges show each item's number.
  const filterKeys = useMemo<(string | null)[]>(
    () => [
      null,
      ...(active > 0 ? ["active"] : []),
      ...(favoriteCount > 0 ? ["favorites"] : []),
      ...tags.map((t) => `tag:${t.name}`),
      ...CLOUD_SECTIONS.flatMap((sec) => [
        ...(hiddenSections.includes(sec.key)
          ? []
          : workspace.integrations.filter((i) => i.cloud === sec.cloud).map((i) => i.id)),
        ...(sec.cloud === CloudKind.CloudAWS ? iamKeys : []),
      ]),
      ...(hiddenCount > 0 ? ["hidden"] : []),
    ],
    [active, favoriteCount, iamKeys, tags, workspace.integrations, hiddenCount, hiddenSections],
  );
  const hintFor = (key: string | null) => {
    const i = filterKeys.indexOf(key);
    return i >= 0 && i < 9 ? hint(String(i + 1)) : undefined;
  };
  // Keyboard shortcuts. ShortcutsDialog lists them; keep the two in step.
  useEffect(() => {
    // The same key closes the dialog it opened.
    const toggle = (kind: "settings" | "import" | "shortcuts") =>
      setDialog((d) => (d?.kind === kind ? null : { kind }));
    const onKey = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey) || e.altKey || e.shiftKey) return;
      // Digits pick the sidebar filters in the order they show, top to bottom.
      if (/^[1-9]$/.test(e.key)) {
        const key = filterKeys[Number(e.key) - 1];
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
  }, [filterKeys]);
  const visibleCount = workspace.sessions.length - hiddenCount;
  // A filter whose sidebar item is gone, or whose section is hidden, falls
  // back to "All sessions". filterKeys holds exactly the items on show.
  const filterExists = filterKeys.includes(chosenFilter);
  const filter = filterExists ? chosenFilter : null;
  // The list is not remounted between filters, so the scroll box goes back to the top itself.
  const scrollBox = useRef<HTMLDivElement>(null);
  const shownFilter = useRef(filter);
  useEffect(() => {
    if (shownFilter.current === filter) return;
    shownFilter.current = filter;
    if (scrollBox.current) scrollBox.current.scrollTop = 0;
  }, [filter]);

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
            : filter === "iam-users"
              ? s.kind === Kind.KindAWSIAMUser
              : filter === "assumed-roles"
                ? s.kind === Kind.KindAWSAssumeRole
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

  return (
    <div className="flex h-full">
      <aside
        style={{ width: sidebarWidth }}
        className="relative flex shrink-0 flex-col border-r bg-sidebar text-sidebar-foreground"
      >
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
              icon={<LayoutList className="size-3.5" />}
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
                accent="var(--color-brand-orange)"
                {...sessionDrop("favorites", "Added to favorites", (id) => api.SetFavorite(id, true))}
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
              // A dropped session tags itself; a dropped tag reorders the list.
              const drop = sessionDrop(key, `Tagged ${tag}`, (id) => api.SetSessionTag(id, tag, true));
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
                      {...drop}
                      accent={t.color || undefined}
                      draggable
                      onDragStart={(e) => {
                        draggingTag.current = tag;
                        e.dataTransfer.setData(TAG_DRAG, tag);
                        e.dataTransfer.effectAllowed = "move";
                      }}
                      onDragEnd={() => {
                        draggingTag.current = null;
                      }}
                      dropLine={dropLine?.tag === tag ? dropLine.side : undefined}
                      onDragOver={(e) => {
                        if (!Array.from(e.dataTransfer.types).includes(TAG_DRAG)) return drop.onDragOver(e);
                        if (draggingTag.current === tag) return;
                        e.preventDefault();
                        const side = lineSide(e);
                        // Dragover fires on every pointer move; keep the old state when nothing changed.
                        setDropLine((cur) => (cur?.tag === tag && cur.side === side ? cur : { tag, side }));
                      }}
                      onDragLeave={() => {
                        drop.onDragLeave();
                        setDropLine((cur) => (cur?.tag === tag ? null : cur));
                      }}
                      onDrop={(e) => {
                        setDropLine(null);
                        const moved = e.dataTransfer.getData(TAG_DRAG);
                        if (!moved) return drop.onDrop(e);
                        e.preventDefault();
                        if (moved === tag) return;
                        // The tag lands on the side of the row the pointer is in.
                        const after = lineSide(e) === "after";
                        // The list shifts once the moved tag leaves its old slot.
                        const from = tags.findIndex((x) => x.name === moved);
                        let to = after ? index + 1 : index;
                        if (from >= 0 && from < to) to -= 1;
                        if (to === from) return;
                        void api.MoveTag(moved, to).catch((err) => toast.error(errorMessage(err)));
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
              // A fragment: a hidden section must not leave an empty box that
              // still takes the nav's spacing.
              <Fragment key={sec.cloud}>
                {showSection(sec.key) && (
                  <div>
                    <div className="flex items-center pr-1">
                      {/* The mark trails the title: rows stay indented under it, so the hierarchy reads. */}
                      <p className="flex flex-1 items-center gap-1.5 px-2 py-1 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
                        {sec.title}
                        <CloudMark cloud={sec.mark} className="size-3" />
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
                )}
                {sec.cloud === CloudKind.CloudAWS && showSection("aws-iam") && (
                  <div>
                    <div className="flex items-center pr-1">
                      <p className="flex flex-1 items-center gap-1.5 px-2 py-1 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
                        AWS IAM
                        <CloudMark cloud="aws" className="size-3" />
                      </p>
                      <span className="flex size-6 items-center justify-center">
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          className="text-muted-foreground"
                          onClick={() => setDialog({ kind: "iam" })}
                          title="Add an IAM user"
                        >
                          <Plus className="size-3.5" />
                        </Button>
                      </span>
                    </div>
                    {iamCount + roleCount === 0 && (
                      <p className="px-2 py-1 text-xs text-muted-foreground/60">None yet</p>
                    )}
                    {iamCount > 0 && (
                      <SideItem
                        active={filter === "iam-users"}
                        onClick={() => setFilter("iam-users")}
                        label="Users"
                        icon={<UserRound className="size-3.5" />}
                        hint={hintFor("iam-users")}
                        count={iamCount}
                      />
                    )}
                    {roleCount > 0 && (
                      <SideItem
                        active={filter === "assumed-roles"}
                        onClick={() => setFilter("assumed-roles")}
                        label="Assumed roles"
                        icon={<Waypoints className="size-3.5" />}
                        hint={hintFor("assumed-roles")}
                        count={roleCount}
                      />
                    )}
                  </div>
                )}
              </Fragment>
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
                      {...sessionDrop("hidden", "Hidden", (id) => api.SetHidden(id, true))}
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
            <HelpMenu />
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
        {/* Drag the edge to resize the sidebar; double-click puts it back. */}
        <span
          role="separator"
          aria-orientation="vertical"
          aria-label="Resize sidebar"
          aria-valuenow={sidebarWidth}
          aria-valuemin={SIDEBAR_MIN}
          aria-valuemax={SIDEBAR_MAX}
          onDoubleClick={() => setSidebarWidth(SIDEBAR_DEFAULT)}
          onMouseDown={(e) => {
            e.preventDefault();
            const startX = e.clientX;
            const startW = sidebarWidth;
            const move = (ev: MouseEvent) =>
              setSidebarWidth(Math.max(SIDEBAR_MIN, Math.min(SIDEBAR_MAX, startW + ev.clientX - startX)));
            const up = () => {
              window.removeEventListener("mousemove", move);
              window.removeEventListener("mouseup", up);
            };
            window.addEventListener("mousemove", move);
            window.addEventListener("mouseup", up);
          }}
          className="no-drag absolute inset-y-0 -right-1 z-10 w-2 cursor-col-resize select-none border-r border-transparent hover:border-border active:border-ring"
        />
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

        {/* The scroll box stays put. Between two lists the table stays mounted
            and only rows that change fade, so the header holds still. An empty
            state crossfades in and out; popLayout lifts the old content out of
            the flow so the new content never jumps. */}
        <div ref={scrollBox} className="relative flex-1 overflow-y-auto">
          <AnimatePresence mode="popLayout" initial={false}>
            <motion.div
              key={sessions.length === 0 ? `empty:${filter ?? "all"}` : "list"}
              initial={{ opacity: 0, y: 6 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: -6 }}
              transition={{ duration: 0.18, ease: [0.25, 0.1, 0.25, 1] }}
              className="flex min-h-full flex-col p-4"
            >
              {(() => {
                const integ = workspace.integrations.find((i) => i.id === filter);
                if (!integ || isLoggedIn(integ) || sessions.length === 0) return null;
                return (
                  <p className="mb-3 px-1 text-xs text-muted-foreground">
                    <span className="font-medium text-foreground">{integ.alias}</span> is signed out. Start a role to
                    sign in.
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
                      onTagClick={(tag) => setFilter(`tag:${tag}`)}
                      shadows={workspace.shadowedProfiles ?? undefined}
                    />
                  </section>
                </div>
              )}
            </motion.div>
          </AnimatePresence>
        </div>
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
          pendingStart.current = null;
        }}
        onDone={(integ) => {
          setFilter(integ.id);
          const id = pendingStart.current;
          if (id) {
            pendingStart.current = null;
            api
              .Start(id, "")
              .then(() => toast.success("Started"))
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
  popped,
  accent,
  dropLine,
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
  /** A drop just landed here; plays the pop. */
  popped?: boolean;
  /** Color of the pop's ring. Defaults to the focus ring. */
  accent?: string;
  /** A dragged item would land above or below this one; draws the line. */
  dropLine?: "before" | "after";
} & React.ComponentProps<"div">) {
  // The count before the drop. The bump waits for the new number instead of
  // playing on the old one and again when the new one arrives.
  const [countAtPop, setCountAtPop] = useState(count);
  if (!popped && countAtPop !== count) setCountAtPop(count);
  return (
    <div
      {...rest}
      style={accent ? ({ "--pop-color": accent } as React.CSSProperties) : undefined}
      className={cn(
        "group relative flex items-center rounded-md pr-1 transition-colors",
        active
          ? "bg-sidebar-accent text-sidebar-accent-foreground"
          : "text-muted-foreground hover:bg-sidebar-accent/60 hover:text-foreground",
        dropping && "bg-sidebar-accent/60 text-foreground ring-1 ring-ring",
        popped && "drop-pop",
      )}
    >
      <button
        type="button"
        onClick={onClick}
        // The dot alone is colour only; the title names the state for hover and assistive tech.
        title={dot ? (dot === "ok" ? "Signed in" : "Signed out") : undefined}
        className="flex min-w-0 flex-1 items-center gap-2 px-2 py-1.5 text-left text-sm"
      >
        {/* Fixed slot for the icon or the dot keeps every label in one column. */}
        <span className="flex size-3.5 shrink-0 items-center justify-center">
          {icon}
          {dot && (
            <span
              aria-hidden
              className={cn("size-1.5 rounded-full", dot === "ok" ? "bg-emerald-400" : "bg-muted-foreground/40")}
            />
          )}
        </span>
        <span className="truncate">{label}</span>
        {count !== undefined && (
          <span
            // The key remounts the badge when the count changes, so the bump
            // plays on the new number.
            key={count}
            className={cn(
              "rounded-full px-1.5 py-px text-[10px] tabular-nums",
              active ? "bg-background/70 text-foreground/70" : "bg-muted text-muted-foreground",
              popped && count !== countAtPop && "count-bump",
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
      {dropLine && (
        <span
          aria-hidden
          data-drop-line={dropLine}
          className={cn(
            "pointer-events-none absolute inset-x-1 h-0.5 rounded-full bg-ring",
            dropLine === "before" ? "-top-px" : "-bottom-px",
          )}
        />
      )}
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
      <div className="flex flex-1 flex-col items-center justify-center text-center">
        <GopherMark className="size-24" autoplay />
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
    <div className="flex flex-1 flex-col items-center justify-center text-center">
      <GopherMark className="size-24" autoplay />
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
