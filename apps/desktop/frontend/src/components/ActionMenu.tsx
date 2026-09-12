import type { ReactNode } from "react";
import {
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
} from "@/components/ui/dropdown-menu";
import {
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuSub,
  ContextMenuSubContent,
  ContextMenuSubTrigger,
} from "@/components/ui/context-menu";

/** One entry of an action list. The same list renders as a dropdown or a context menu. */
export type Action =
  | "separator"
  | { label: string; icon?: ReactNode; onSelect: () => void; destructive?: boolean }
  | { label: string; icon?: ReactNode; items: Action[] };

export function ActionItems({ actions, menu }: { actions: Action[]; menu: "dropdown" | "context" }) {
  const Item = menu === "dropdown" ? DropdownMenuItem : ContextMenuItem;
  const Separator = menu === "dropdown" ? DropdownMenuSeparator : ContextMenuSeparator;
  const Sub = menu === "dropdown" ? DropdownMenuSub : ContextMenuSub;
  const SubTrigger = menu === "dropdown" ? DropdownMenuSubTrigger : ContextMenuSubTrigger;
  const SubContent = menu === "dropdown" ? DropdownMenuSubContent : ContextMenuSubContent;
  return (
    <>
      {actions.map((a, i) => {
        if (a === "separator") return <Separator key={`sep-${i}`} />;
        if ("items" in a) {
          return (
            <Sub key={a.label}>
              <SubTrigger>
                {a.icon} {a.label}
              </SubTrigger>
              <SubContent className="min-w-44">
                <ActionItems actions={a.items} menu={menu} />
              </SubContent>
            </Sub>
          );
        }
        return (
          <Item key={a.label} variant={a.destructive ? "destructive" : "default"} onSelect={a.onSelect}>
            {a.icon} {a.label}
          </Item>
        );
      })}
    </>
  );
}
