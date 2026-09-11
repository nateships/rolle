import type { ReactNode } from "react";
import { DropdownMenuItem, DropdownMenuSeparator } from "@/components/ui/dropdown-menu";
import { ContextMenuItem, ContextMenuSeparator } from "@/components/ui/context-menu";

/** One entry of an action list. The same list renders as a dropdown or a context menu. */
export type Action = "separator" | { label: string; icon?: ReactNode; onSelect: () => void; destructive?: boolean };

export function ActionItems({ actions, menu }: { actions: Action[]; menu: "dropdown" | "context" }) {
  const Item = menu === "dropdown" ? DropdownMenuItem : ContextMenuItem;
  const Separator = menu === "dropdown" ? DropdownMenuSeparator : ContextMenuSeparator;
  return (
    <>
      {actions.map((a, i) =>
        a === "separator" ? (
          <Separator key={`sep-${i}`} />
        ) : (
          <Item key={a.label} variant={a.destructive ? "destructive" : "default"} onSelect={a.onSelect}>
            {a.icon} {a.label}
          </Item>
        ),
      )}
    </>
  );
}
