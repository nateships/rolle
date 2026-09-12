import { useEffect, useState } from "react";
import { isMac } from "@/components/dialogs/ShortcutsDialog";

/**
 * True while the shortcut modifier (⌘ on macOS, Ctrl elsewhere) has been held
 * for longer than delay. A quick combo such as ⌘C cancels the pending show,
 * so the badges appear only when the user pauses. Once shown they stay while
 * the modifier is down, so ⌘3 then ⌘1 keeps them on screen.
 */
export function useModifierHeld(delay = 350) {
  const [held, setHeld] = useState(false);
  useEffect(() => {
    let timer: ReturnType<typeof setTimeout> | undefined;
    let shown = false;
    const clear = () => {
      if (timer) clearTimeout(timer);
      timer = undefined;
      shown = false;
      setHeld(false);
    };
    const onDown = (e: KeyboardEvent) => {
      const mod = isMac ? e.key === "Meta" : e.key === "Control";
      if (!mod) {
        // A combo before the delay is a shortcut, not a pause.
        if (!shown) clear();
        return;
      }
      if (!e.repeat && !timer) {
        timer = setTimeout(() => {
          shown = true;
          setHeld(true);
        }, delay);
      }
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
