import { useEffect, useMemo, useState } from "react";
import { motion } from "motion/react";
import {
  ArrowDown,
  ArrowUp,
  ChevronRight,
  ChevronsDownUp,
  ChevronsUpDown,
  Columns3,
  Copy,
  Eye,
  EyeOff,
} from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { ContextMenu, ContextMenuContent, ContextMenuTrigger } from "@/components/ui/context-menu";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { ActionItems, type Action } from "@/components/ActionMenu";
import { copyText } from "@/lib/clipboard";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { CloudGlyph } from "@/components/Brand";
import { SessionRow } from "./SessionRow";
import { COLUMN_KEYS, DEFAULT_COLUMNS, type ColumnKey, type Columns } from "./columns";
import { api, errorMessage, Kind, Status, type Integration, type Session, type Workspace } from "@/lib/api";
import { cloudOf } from "@/lib/format";
import { cn } from "@/lib/utils";

const cloudOrder = { aws: 0, azure: 1, gcp: 2 } as const;

/** An account with its SSO roles, or a session that stands alone. */
type Row =
  | { key: string; label: string; group: true; integrationId: string; accountId: string; sessions: Session[] }
  | { key: string; label: string; group: false; session: Session };

/** SSO roles group under their account; other sessions stand alone. A flat list puts every session alone. */
export function groupSessions(sessions: Session[], flat = false): Row[] {
  const groups = new Map<string, Row & { group: true }>();
  const rows: Row[] = [];
  for (const s of sessions) {
    if (flat || s.kind !== Kind.KindAWSSSORole || !s.aws?.accountId) {
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
/** A column never leaves this range. Cells clip what does not fit. */
const MIN_WIDTHS: ColumnWidths = { profile: 70, region: 90, state: 120 };
const MAX_WIDTH = 320;
/** The Session column takes the rest of the row. Keep this much of it for a name. */
const SESSION_MIN = 160;
const COLUMN_LABELS = { session: "Session", profile: "Profile", region: "Region", state: "State" } as const;
const WIDTHS_KEY = "rolle.columns";
const COLUMNS_KEY = "rolle.columns.shown";
const COLLAPSED_KEY = "rolle.collapsed";

const clamp = (n: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, n));

function clampWidths(w: ColumnWidths): ColumnWidths {
  const fit = (k: keyof ColumnWidths) => clamp(w[k], MIN_WIDTHS[k], MAX_WIDTH);
  return { profile: fit("profile"), region: fit("region"), state: fit("state") };
}

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
  // Older builds stored narrower columns. Pull stored widths into range.
  const [widths, setWidths] = useState<ColumnWidths>(() => clampWidths(readJSON(WIDTHS_KEY, DEFAULT_WIDTHS)));
  useEffect(() => {
    try {
      localStorage.setItem(WIDTHS_KEY, JSON.stringify(widths));
    } catch {
      /* storage is optional */
    }
  }, [widths]);
  return [widths, setWidths] as const;
}

/** Which optional columns show, kept between launches. */
function useColumns() {
  const [columns, setColumns] = useState<Columns>(() => readJSON(COLUMNS_KEY, DEFAULT_COLUMNS));
  useEffect(() => {
    try {
      localStorage.setItem(COLUMNS_KEY, JSON.stringify(columns));
    } catch {
      /* storage is optional */
    }
  }, [columns]);
  return [columns, setColumns] as const;
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
  const [columns, setColumns] = useColumns();
  // Settings → Appearance: one row per role, and one line per row.
  const flat = !!workspace.settings?.flatList;
  const compact = !!workspace.settings?.compact;
  const rows = useMemo(() => sortRows(groupSessions(sessions, flat), sort), [sessions, sort, flat]);
  // The optional columns on show, in table order.
  const shown = COLUMN_KEYS.filter((k) => columns[k]);
  // A hidden column stops sorting the table; the header could not show it.
  const showColumn = (key: ColumnKey, on: boolean) => {
    setColumns({ ...columns, [key]: on });
    if (!on && sort?.key === key) setSort(null);
  };

  // A header click sorts ascending, again descending, a third time clears it.
  function sortButton(key: NonNullable<Sort>["key"]) {
    const on = sort?.key === key;
    const Arrow = !on ? ChevronsUpDown : sort.dir === "asc" ? ArrowUp : ArrowDown;
    return (
      <button
        type="button"
        onClick={() => setSort(!on ? { key, dir: "asc" } : sort.dir === "asc" ? { key, dir: "desc" } : null)}
        className="group -mx-1 inline-flex items-center gap-1 rounded px-1 hover:text-foreground"
      >
        {COLUMN_LABELS[key]}
        <Arrow className={cn("size-3", !on && "opacity-0 group-hover:opacity-60")} />
      </button>
    );
  }
  const ariaSort = (key: NonNullable<Sort>["key"]) =>
    sort?.key === key ? (sort.dir === "asc" ? "ascending" : "descending") : "none";

  // A divider between two columns. Dragging it moves the boundary under the
  // pointer. The left column grows by the amount the right column loses, so no
  // other column moves. Both columns stay in range. The Session column has no
  // fixed width, so its divider (no `left`) only sets Profile and keeps
  // SESSION_MIN of Session. Double-click puts both columns back.
  function resizer(right: keyof ColumnWidths, left?: keyof ColumnWidths) {
    return (
      <span
        role="separator"
        aria-orientation="vertical"
        aria-label={`Resize ${COLUMN_LABELS[left ?? "session"]} column`}
        aria-valuenow={left && widths[left]}
        onDoubleClick={() =>
          onWidths({ ...widths, ...(left && { [left]: DEFAULT_WIDTHS[left] }), [right]: DEFAULT_WIDTHS[right] })
        }
        onMouseDown={(e) => {
          e.preventDefault();
          const startX = e.clientX;
          const start = widths;
          // The bounds on dx do not change during a drag. Compute them once.
          let lo = start[right] - MAX_WIDTH;
          let hi = start[right] - MIN_WIDTHS[right];
          if (left) {
            lo = Math.max(lo, MIN_WIDTHS[left] - start[left]);
            hi = Math.min(hi, MAX_WIDTH - start[left]);
          } else {
            // The header cell is as wide as the Session column. An unmeasured
            // header (no layout yet) puts no ceiling on Profile. A column that
            // is already too narrow can still grow, so the floor is at most 0.
            const session = e.currentTarget.parentElement?.clientWidth || Infinity;
            lo = Math.max(lo, Math.min(0, SESSION_MIN - session));
          }
          const move = (ev: MouseEvent) => {
            const dx = clamp(ev.clientX - startX, lo, hi);
            onWidths({ ...start, ...(left && { [left]: start[left] + dx }), [right]: start[right] - dx });
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

  // The actions column is pinned to the right edge, so the last divider only marks the boundary.
  const divider = <span aria-hidden="true" className="absolute inset-y-2 -right-px w-px bg-border/60" />;

  return (
    <div className="overflow-hidden rounded-lg border bg-card">
      {/* Cells clip what does not fit. `clip` (not `hidden`) keeps a cell from scrolling when a clipped child takes focus. */}
      <Table className={cn("table-fixed [&_td]:overflow-clip", compact && "[&_td]:py-1")}>
        <colgroup>
          <col style={{ width: 84 }} />
          <col />
          {shown.map((k) => (
            <col key={k} style={{ width: widths[k] }} />
          ))}
          <col style={{ width: 184 }} />
        </colgroup>
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            <TableHead />
            <TableHead aria-sort={ariaSort("session")} className="relative">
              {sortButton("session")}
              {shown.length > 0 ? resizer(shown[0]) : divider}
            </TableHead>
            {shown.map((k, i) => (
              <TableHead key={k} aria-sort={ariaSort(k)} className="relative">
                {sortButton(k)}
                {i + 1 < shown.length ? resizer(shown[i + 1], k) : divider}
              </TableHead>
            ))}
            <TableHead className="text-right">
              <span className="inline-flex items-center gap-1">
                Actions
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button
                      variant="ghost"
                      size="icon-xs"
                      className="text-muted-foreground"
                      aria-label="Choose columns"
                    >
                      <Columns3 className="size-3.5" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" className="min-w-40">
                    <DropdownMenuLabel>Columns</DropdownMenuLabel>
                    <DropdownMenuSeparator />
                    {COLUMN_KEYS.map((k) => (
                      <DropdownMenuCheckboxItem
                        key={k}
                        checked={columns[k]}
                        onCheckedChange={(on) => showColumn(k, on)}
                        // The menu stays open, so several columns toggle in one go.
                        onSelect={(e) => e.preventDefault()}
                      >
                        {COLUMN_LABELS[k]}
                      </DropdownMenuCheckboxItem>
                    ))}
                  </DropdownMenuContent>
                </DropdownMenu>
              </span>
            </TableHead>
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
                  columns={columns}
                  onNeedsLogin={onNeedsLogin}
                  onTagClick={onTagClick}
                  shadows={shadows}
                />,
              ];
            const open = searching || !collapsed.includes(r.key);
            const out = [
              <AccountRow
                key={r.key}
                row={r}
                open={open}
                compact={compact}
                span={shown.length + 2}
                onToggle={() => toggle(r.key)}
              />,
            ];
            if (open)
              out.push(
                ...r.sessions.map((s) => (
                  <SessionRow
                    key={s.id}
                    session={s}
                    workspace={workspace}
                    columns={columns}
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

function AccountRow({
  row,
  open,
  compact,
  span,
  onToggle,
}: {
  row: Row & { group: true };
  open: boolean;
  /** One line: the name, the account ID, and the role count side by side. */
  compact: boolean;
  /** How many columns the label spans: Session, the optional columns, and Actions. */
  span: number;
  onToggle: () => void;
}) {
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
          <TableCell colSpan={span}>
            <div className={cn("flex items-center gap-2.5", compact && "gap-2")}>
              <CloudGlyph cloud="aws" className={compact ? "size-6 p-1" : undefined} />
              <div className={cn("min-w-0", compact && "flex items-baseline gap-2")}>
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
