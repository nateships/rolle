import { useEffect, useMemo, useState } from "react";
import { motion } from "motion/react";
import { ChevronRight } from "lucide-react";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { CloudGlyph } from "@/components/Brand";
import { SessionRow } from "./SessionRow";
import { Kind, Status, type Integration, type Session, type Workspace } from "@/lib/api";
import { cloudOf } from "@/lib/format";
import { cn } from "@/lib/utils";

const cloudOrder = { aws: 0, azure: 1, gcp: 2 } as const;

/** An account with its SSO roles, or a session that stands alone. */
type Row =
  | { key: string; label: string; group: true; accountId: string; sessions: Session[] }
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
        label: s.name.split("/")[0] || s.aws.accountId,
        group: true,
        accountId: s.aws.accountId,
        sessions: [],
      };
      groups.set(key, g);
      rows.push(g);
    }
    g.sessions.push(s);
  }
  for (const g of groups.values())
    g.sessions.sort((a, b) => (a.aws?.roleName ?? a.name).localeCompare(b.aws?.roleName ?? b.name));
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

type TableProps = {
  sessions: Session[];
  workspace: Workspace;
  /** A search is in progress: every account shows its matching roles. */
  searching?: boolean;
  /** Favorites list rows flat, without account groups. */
  flat?: boolean;
  widths: ColumnWidths;
  onWidths: (w: ColumnWidths) => void;
  onNeedsLogin: (i: Integration, startId?: string) => void;
};

export function SessionTable({ sessions, workspace, searching, flat, widths, onWidths, onNeedsLogin }: TableProps) {
  const [collapsed, toggle] = useCollapsed();
  const rows = useMemo(
    () =>
      flat
        ? sessions.map<Row>((s) => ({ key: s.id, label: s.name, group: false, session: s }))
        : groupSessions(sessions),
    [sessions, flat],
  );

  function resizer(col: keyof ColumnWidths) {
    return (
      <span
        role="separator"
        aria-orientation="vertical"
        onMouseDown={(e) => {
          e.preventDefault();
          const startX = e.clientX;
          const startW = widths[col];
          const move = (ev: MouseEvent) =>
            onWidths({ ...widths, [col]: Math.max(70, Math.min(320, startW + ev.clientX - startX)) });
          const up = () => {
            window.removeEventListener("mousemove", move);
            window.removeEventListener("mouseup", up);
          };
          window.addEventListener("mousemove", move);
          window.addEventListener("mouseup", up);
        }}
        className="absolute inset-y-0 right-0 w-2 cursor-col-resize select-none border-r border-transparent hover:border-border active:border-ring"
      />
    );
  }

  return (
    <div className="overflow-hidden rounded-lg border bg-card">
      <Table className="table-fixed">
        <colgroup>
          <col style={{ width: 56 }} />
          <col />
          <col style={{ width: widths.profile }} />
          <col style={{ width: widths.region }} />
          <col style={{ width: widths.state }} />
          <col style={{ width: 208 }} />
        </colgroup>
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            <TableHead />
            <TableHead>Session</TableHead>
            <TableHead className="relative">Profile{resizer("profile")}</TableHead>
            <TableHead className="relative">Region{resizer("region")}</TableHead>
            <TableHead className="relative">State{resizer("state")}</TableHead>
            <TableHead />
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.flatMap((r) => {
            if (!r.group)
              return [<SessionRow key={r.key} session={r.session} workspace={workspace} onNeedsLogin={onNeedsLogin} />];
            const open = searching || !collapsed.includes(r.key);
            const out = [<AccountRow key={r.key} row={r} open={open} onToggle={() => toggle(r.key)} />];
            if (open)
              out.push(
                ...r.sessions.map((s) => (
                  <SessionRow key={s.id} session={s} workspace={workspace} nested onNeedsLogin={onNeedsLogin} />
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
  return (
    <motion.tr
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      transition={{ duration: 0.15 }}
      onClick={onToggle}
      className={cn(
        "cursor-pointer select-none border-b bg-muted/20 transition-colors hover:bg-muted/50",
        active > 0 && "bg-emerald-500/[0.04]",
      )}
    >
      <TableCell className="pr-0 text-center">
        <ChevronRight
          className={cn("inline-block size-4 text-muted-foreground transition-transform", open && "rotate-90")}
        />
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
  );
}
