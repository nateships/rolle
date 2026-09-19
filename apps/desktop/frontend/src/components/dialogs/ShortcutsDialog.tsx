import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { cn } from "@/lib/utils";

/** True on macOS, where the modifier is ⌘. Everywhere else it is Ctrl. */
export const isMac = /Mac/.test(navigator.platform) || /Mac OS/.test(navigator.userAgent);

const MOD = isMac ? "⌘" : "Ctrl";

/** One shortcut as a badge reads: ⌘1 on macOS, Ctrl+1 elsewhere. */
export const combo = (key: string) => (isMac ? `⌘${key}` : `Ctrl+${key}`);

/** Every shortcut the dashboard handles, in the order the sheet lists them. */
export const SHORTCUTS: { keys: string[]; label: string }[] = [
  { keys: [MOD, ","], label: "Settings" },
  { keys: [MOD, "F"], label: "Search sessions" },
  { keys: [MOD, "I"], label: "Import from this machine" },
  { keys: [MOD, "1…9"], label: "Sidebar filters, top to bottom" },
  { keys: [MOD, "\\"], label: "Show or hide the sidebar" },
  { keys: ["Esc"], label: "Clear the search, close a dialog" },
  { keys: [MOD, "/"], label: "Keyboard shortcuts" },
];

export function Key({ children, className }: { children: string; className?: string }) {
  return (
    <kbd
      className={cn(
        "inline-flex h-6 min-w-6 items-center justify-center rounded-md border bg-muted px-1.5 font-sans text-[11px] font-medium text-foreground shadow-[inset_0_-1px_0_var(--color-border)]",
        className,
      )}
    >
      {children}
    </kbd>
  );
}

export function ShortcutsDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>Keyboard shortcuts</DialogTitle>
          <DialogDescription>Work the dashboard without the mouse.</DialogDescription>
        </DialogHeader>
        <ul className="divide-y text-sm">
          {SHORTCUTS.map((s) => (
            <li key={s.label} className="flex items-center justify-between gap-4 py-2">
              <span>{s.label}</span>
              <span className="flex items-center gap-1">
                {s.keys.map((k) => (
                  <Key key={k}>{k}</Key>
                ))}
              </span>
            </li>
          ))}
        </ul>
      </DialogContent>
    </Dialog>
  );
}
