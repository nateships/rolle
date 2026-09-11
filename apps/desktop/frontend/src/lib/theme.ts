export type Theme = "system" | "light" | "dark";
const KEY = "rolle-theme";

/** Resolve a theme preference against the OS setting. */
export function resolveTheme(pref: Theme): "light" | "dark" {
  if (pref === "light" || pref === "dark") return pref;
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

/** Apply a preference to the document and remember it for the next launch. */
export function applyTheme(pref: Theme) {
  document.documentElement.classList.toggle("dark", resolveTheme(pref) === "dark");
  try {
    localStorage.setItem(KEY, pref);
  } catch {
    /* storage unavailable */
  }
}

/** Last applied preference, used before settings load to avoid a flash. */
export function cachedTheme(): Theme {
  try {
    const v = localStorage.getItem(KEY);
    if (v === "light" || v === "dark" || v === "system") return v;
  } catch {
    /* storage unavailable */
  }
  return "dark";
}

/** Re-apply when the OS theme flips and the preference is "system". */
export function watchSystemTheme(getPref: () => Theme) {
  const mq = window.matchMedia("(prefers-color-scheme: dark)");
  const onChange = () => {
    if (getPref() === "system") applyTheme("system");
  };
  mq.addEventListener("change", onChange);
  return () => mq.removeEventListener("change", onChange);
}
