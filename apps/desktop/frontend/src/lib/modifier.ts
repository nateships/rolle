import { useEffect, useState } from "react";
import { isMac } from "@/components/dialogs/ShortcutsDialog";

/**
 * True while the shortcut modifier (⌘ on macOS, Ctrl elsewhere) is held on its
 * own for longer than delay. A combo such as ⌘C cancels it, so the badges
 * appear only when the user pauses to look.
 */
export function useModifierHeld(delay = 350) {
  const [held, setHeld] = useState(false);
  useEffect(() => {
    let timer: ReturnType<typeof setTimeout> | undefined;
    const clear = () => {
      if (timer) clearTimeout(timer);
      timer = undefined;
      setHeld(false);
    };
    const onDown = (e: KeyboardEvent) => {
      const mod = isMac ? e.key === "Meta" : e.key === "Control";
      if (!mod) {
        clear();
        return;
      }
      if (!e.repeat && !timer) timer = setTimeout(() => setHeld(true), delay);
    };
    const onUp = (e: KeyboardEvent) => {
      if (e.key === "Meta" || e.key === "Control") clear();
    };
    window.addEventListener("keydown", onDown);
    window.addEventListener("keyup", onUp);
    window.addEventListener("blur", clear);
    return () => {
      window.removeEventListener("keydown", onDown);
      window.removeEventListener("keyup", onUp);
      window.removeEventListener("blur", clear);
      clear();
    };
  }, [delay]);
  return held;
}
