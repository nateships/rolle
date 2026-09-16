import { useEffect, useMemo, useState } from "react";
import { motion } from "motion/react";
import { ArrowDown, ArrowUp, ChevronRight, ChevronsDownUp, ChevronsUpDown, Copy, Eye, EyeOff } from "lucide-react";
import { toast } from "sonner";
import { ContextMenu, ContextMenuContent, ContextMenuTrigger } from "@/components/ui/context-menu";
import { ActionItems, type Action } from "@/components/ActionMenu";
import { copyText } from "@/lib/clipboard";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { CloudGlyph } from "@/components/Brand";
import { SessionRow } from "./SessionRow";
import { api, errorMessage, Kind, Status, type Integration, type Session, type Workspace } from "@/lib/api";
import { cloudOf } from "@/lib/format";
import { cn } from "@/lib/utils";

const cloudOrder = { aws: 0, azure: 1, gcp: 2 } as const;

/** An account with its SSO roles, or a session that stands alone. */
type Row =
  | { key: string; label: string; group: true; integrationId: string; accountId: string; sessions: Session[] }
  | { key: string; label: string; group: false; session: Session };

/** SSO roles group under their account; other sessions stand alone. */
export function groupSessions(sessions: Session[]): Row[] {
  const groups = new Map<string, Row & { group: true }>();
  const rows: Row[] = [];
  for (const s of sessions) {
    if (s.kind !== Kind.KindAWSSSORole || !s.aws?.accountId) {
      rows.push({ key: s.id, label: s.name, group: false, session: s });
      continue;
    }
    const key = `${s.integrationId}:${s.aws.accountId}`;
    let g = groups.get(key);
    if (!g) {
      g = {
        key,
        label: s.aws.accountId,
        group: true,
        integrationId: s.integrationId ?? "",
        accountId: s.aws.accountId,
        sessions: [],
      };
      groups.set(key, g);
      rows.push(g);
    }
    g.sessions.push(s);
  }
  for (const g of groups.values()) {
    // The account name is the prefix of any role that still carries its "account/role" name.
    g.label =
      g.sessions
        .map((s) => s.name)
        .find((n) => n.includes("/"))
        ?.split("/")[0] || g.accountId;
    g.sessions.sort((a, b) => (a.aws?.roleName ?? a.name).localeCompare(b.aws?.roleName ?? b.name));
  }
  const cloud = (r: Row) => cloudOf(r.group ? r.sessions[0].kind : r.session.kind);
  return rows.sort((a, b) => cloudOrder[cloud(a)] - cloudOrder[cloud(b)] || a.label.localeCompare(b.label));
}

/** Column widths in pixels for the resizable columns. */
export type ColumnWidths = { profile: number; region: number; state: number };
const DEFAULT_WIDTHS: ColumnWidths = { profile: 150, region: 130, state: 130 };
const WIDTHS_KEY = "rolle.columns";
const COLLAPSED_KEY = "rolle.collapsed";

function readJSON<T>(key: string, fallback: T): T {
  try {
    const raw = localStorage.getItem(key);
    return raw ? { ...fallback, ...JSON.parse(raw) } : fallback;
  } catch {
    return fallback;
  }
}

/** Column widths shared by every table on the page, kept between launches. */
export function useColumnWidths() {
  const [widths, setWidths] = useState<ColumnWidths>(() => readJSON(WIDTHS_KEY, DEFAULT_WIDTHS));
  useEffect(() => {
    try {
      localStorage.setItem(WIDTHS_KEY, JSON.stringify(widths));
    } catch {
      /* storage is optional */
    }
  }, [widths]);
  return [widths, setWidths] as const;
}

function useCollapsed() {
  const [collapsed, setCollapsed] = useState<string[]>(
    () => readJSON<{ keys: string[] }>(COLLAPSED_KEY, { keys: [] }).keys,
  );
  useEffect(() => {
    try {
      localStorage.setItem(COLLAPSED_KEY, JSON.stringify({ keys: collapsed }));
    } catch {
      /* storage is optional */
    }
  }, [collapsed]);
  const toggle = (key: string) => setCollapsed((c) => (c.includes(key) ? c.filter((k) => k !== key) : [...c, key]));
  return [collapsed, toggle] as const;
}

/** A column to sort by and the direction; null keeps the default order. */
export type Sort = { key: "session" | "profile" | "region" | "state"; dir: "asc" | "desc" } | null;
const SORT_KEY = "rolle.sort";

function useSort() {
  const [sort, setSort] = useState<Sort>(() => readJSON<{ sort: Sort }>(SORT_KEY, { sort: null }).sort);
  useEffect(() => {
    try {
      localStorage.setItem(SORT_KEY, JSON.stringify({ sort }));
    } catch {
      /* storage is optional */
    }
  }, [sort]);
  return [sort, setSort] as const;
}

/**
 * Orders rows by a column. An account row takes the place of its first role in
 * that order, and its roles follow the same order. Sorting by state puts the
 * active sessions first, soonest to expire on top, then the rest by name.
 */
export function sortRows(rows: Row[], sort: Sort): Row[] {
  if (!sort) return rows;
  const dir = sort.dir === "asc" ? 1 : -1;
  const text = (s: Session) =>
    sort.key === "profile"
      ? s.aws
        ? s.aws.profile || "default"
        : ""
      : sort.key === "region"
        ? (s.region ?? "")
        : s.name;
  const due = (s: Session) =>
    s.status === Status.StatusActive ? Date.parse(s.expires ?? "") || Number.MAX_SAFE_INTEGER : Number.MAX_VALUE;
  const cmp = (a: Session, b: Session) =>
    (sort.key === "state" ? due(a) - due(b) : text(a).localeCompare(text(b))) || a.name.localeCompare(b.name);
  const sorted = rows.map((r) => (r.group ? { ...r, sessions: [...r.sessions].sort((a, b) => cmp(a, b) * dir) } : r));
  const first = (r: Row) => (r.group ? r.sessions[0] : r.session);
  return sorted.sort((a, b) => cmp(first(a), first(b)) * dir);
}

type TableProps = {
  sessions: Session[];
  workspace: Workspace;
  /** A search is in progress: every account shows its matching roles. */
  searching?: boolean;
  widths: ColumnWidths;
  onWidths: (w: ColumnWidths) => void;
  onNeedsLogin: (i: Integration, startId?: string) => void;
  /** A tag chip on a row was clicked; the dashboard filters by it. */
  onTagClick?: (tag: string) => void;
  /** Profile name to the file whose static keys shadow it. */
  shadows?: Record<string, string | undefined>;
};

export function SessionTable({
  sessions,
  workspace,
  searching,
  widths,
  onWidths,
  onNeedsLogin,
  onTagClick,
  shadows,
}: TableProps) {
  const [collapsed, toggle] = useCollapsed();
  const [sort, setSort] = useSort();
  const rows = useMemo(() => sortRows(groupSessions(sessions), sort), [sessions, sort]);

  // A header click sorts ascending, again descending, a third time clears it.
  function sortButton(key: NonNullable<Sort>["key"], label: string) {
    const on = sort?.key === key;
    const Arrow = !on ? ChevronsUpDown : sort.dir === "asc" ? ArrowUp : ArrowDown;
    return (
      <button
        type="button"
        onClick={() => setSort(!on ? { key, dir: "asc" } : sort.dir === "asc" ? { key, dir: "desc" } : null)}
        className="group -mx-1 inline-flex items-center gap-1 rounded px-1 hover:text-foreground"
      >
        {label}
        <Arrow className={cn("size-3", !on && "opacity-0 group-hover:opacity-60")} />
      </button>
    );
  }
  const ariaSort = (key: NonNullable<Sort>["key"]) =>
    sort?.key === key ? (sort.dir === "asc" ? "ascending" : "descending") : "none";

  // A divider between two columns. Dragging it moves the boundary under the
  // pointer: the column on its left grows by what the column on its right
  // gives up, so nothing else shifts. The Session column is the flexible one,
  // so its divider only sets Profile. Double-click puts both columns back.
  const MIN = 70;
  const MAX = 320;
  function resizer(left: keyof ColumnWidths | "session", right: keyof ColumnWidths) {
    const name = { session: "Session", profile: "Profile", region: "Region", state: "State" };
    return (
      <span
        role="separator"
        aria-orientation="vertical"
        aria-label={`Resize ${name[left]} column`}
        aria-valuenow={left === "session" ? undefined : widths[left]}
        onDoubleClick={() =>
          onWidths({
            ...widths,
            ...(left !== "session" && { [left]: DEFAULT_WIDTHS[left] }),
            [right]: DEFAULT_WIDTHS[right],
          })
        }
        onMouseDown={(e) => {
          e.preventDefault();
          const startX = e.clientX;
          const start = { ...widths };
          const move = (ev: MouseEvent) => {
            let dx = ev.clientX - startX;
            // The right column gives what the left one takes; both stay in range.
            dx = Math.max(start[right] - MAX, Math.min(start[right] - MIN, dx));
            if (left !== "session") dx = Math.max(MIN - start[left], Math.min(MAX - start[left], dx));
            onWidths({
              ...start,
              ...(left !== "session" && { [left]: start[left] + dx }),
              [right]: start[right] - dx,
            });
          };
          const up = () => {
            window.removeEventListener("mousemove", move);
            window.removeEventListener("mouseup", up);
          };
          window.addEventListener("mousemove", move);
          window.addEventListener("mouseup", up);
        }}
        className="absolute inset-y-2 -right-1.5 w-3 cursor-col-resize select-none before:absolute before:inset-y-0 before:left-1/2 before:w-px before:bg-border/60 before:transition-colors hover:before:w-0.5 hover:before:bg-border active:before:w-0.5 active:before:bg-ring"
      />
    );
  }

  return (
    <div className="overflow-hidden rounded-lg border bg-card">
      <Table className="table-fixed">
        <colgroup>
          <col style={{ width: 84 }} />
          <col />
          <col style={{ width: widths.profile }} />
          <col style={{ width: widths.region }} />
          <col style={{ width: widths.state }} />
          <col style={{ width: 184 }} />
        </colgroup>
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            <TableHead />
            <TableHead aria-sort={ariaSort("session")} className="relative">
              {sortButton("session", "Session")}
              {resizer("session", "profile")}
            </TableHead>
            <TableHead aria-sort={ariaSort("profile")} className="relative">
              {sortButton("profile", "Profile")}
              {resizer("profile", "region")}
            </TableHead>
            <TableHead aria-sort={ariaSort("region")} className="relative">
              {sortButton("region", "Region")}
              {resizer("region", "state")}
            </TableHead>
            <TableHead aria-sort={ariaSort("state")}>{sortButton("state", "State")}</TableHead>
            <TableHead className="text-right">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.flatMap((r) => {
            if (!r.group)
              return [
                <SessionRow
                  key={r.key}
                  session={r.session}
                  workspace={workspace}
                  onNeedsLogin={onNeedsLogin}
                  onTagClick={onTagClick}
                  shadows={shadows}
                />,
              ];
            const open = searching || !collapsed.includes(r.key);
            const out = [<AccountRow key={r.key} row={r} open={open} onToggle={() => toggle(r.key)} />];
            if (open)
              out.push(
                ...r.sessions.map((s) => (
                  <SessionRow
                    key={s.id}
                    session={s}
                    workspace={workspace}
                    nested
                    onNeedsLogin={onNeedsLogin}
                    onTagClick={onTagClick}
                    shadows={shadows}
                  />
                )),
              );
            return out;
          })}
        </TableBody>
      </Table>
    </div>
  );
}

function AccountRow({ row, open, onToggle }: { row: Row & { group: true }; open: boolean; onToggle: () => void }) {
  const active = row.sessions.filter((s) => s.status === Status.StatusActive).length;
  const allHidden = row.sessions.every((s) => s.hidden);
  const actions: Action[] = [
    { label: open ? "Collapse" : "Expand", icon: open ? <ChevronsDownUp /> : <ChevronsUpDown />, onSelect: onToggle },
    {
      label: "Copy account ID",
      icon: <Copy />,
      onSelect: () =>
        void copyText(row.accountId)
          .then(() => toast.success("Copied", { description: row.accountId }))
          .catch((e) => toast.error(errorMessage(e))),
    },
    {
      label: allHidden ? "Unhide account" : "Hide account",
      icon: allHidden ? <Eye /> : <EyeOff />,
      onSelect: () =>
        void api
          .SetAccountHidden(row.integrationId, row.accountId, !allHidden)
          .catch((e) => toast.error(errorMessage(e))),
    },
  ];
  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>
        <motion.tr
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ duration: 0.15 }}
          onClick={onToggle}
          className={cn(
            "cursor-pointer select-none border-b bg-muted/40 transition-colors hover:bg-muted/60",
            active > 0 && "bg-emerald-500/[0.04]",
          )}
        >
          <TableCell className="pr-0">
            {/* The row toggles on click; this button gives the keyboard the same control. */}
            <button
              type="button"
              aria-expanded={open}
              aria-label={`${open ? "Collapse" : "Expand"} ${row.label}`}
              onClick={(e) => {
                e.stopPropagation();
                onToggle();
              }}
              className="ml-1.5 inline-flex rounded-sm text-muted-foreground focus-visible:outline-2 focus-visible:outline-ring"
            >
              <ChevronRight className={cn("size-4 transition-transform", open && "rotate-90")} />
            </button>
          </TableCell>
          <TableCell colSpan={5}>
            <div className="flex items-center gap-2.5">
              <CloudGlyph cloud="aws" />
              <div className="min-w-0">
                <p className="truncate text-sm font-medium">{row.label}</p>
                <p className="truncate font-mono text-[11px] text-muted-foreground">
                  {row.accountId} · {row.sessions.length} {row.sessions.length === 1 ? "role" : "roles"}
                  {active > 0 ? ` · ${active} active` : ""}
                </p>
              </div>
            </div>
          </TableCell>
        </motion.tr>
      </ContextMenuTrigger>
      <ContextMenuContent className="min-w-48">
        <ActionItems actions={actions} menu="context" />
      </ContextMenuContent>
    </ContextMenu>
  );
}
