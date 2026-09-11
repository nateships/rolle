import { beforeEach, describe, expect, it, vi } from "vitest";
import { applyTheme, cachedTheme, resolveTheme, watchSystemTheme } from "@/lib/theme";

function osPrefersDark(dark: boolean) {
  const listeners = new Set<() => void>();
  const mq = {
    matches: dark,
    addEventListener: (_: string, cb: () => void) => listeners.add(cb),
    removeEventListener: (_: string, cb: () => void) => listeners.delete(cb),
  } as unknown as MediaQueryList;
  vi.mocked(window.matchMedia).mockReturnValue(mq);
  return { fire: () => listeners.forEach((l) => l()), listeners };
}

describe("resolveTheme", () => {
  beforeEach(() => document.documentElement.classList.remove("dark"));

  it("returns explicit preferences unchanged", () => {
    osPrefersDark(true);
    expect(resolveTheme("light")).toBe("light");
    osPrefersDark(false);
    expect(resolveTheme("dark")).toBe("dark");
  });

  it("follows the OS for system", () => {
    osPrefersDark(true);
    expect(resolveTheme("system")).toBe("dark");
    osPrefersDark(false);
    expect(resolveTheme("system")).toBe("light");
  });

  it("applyTheme toggles the dark class and remembers the preference", () => {
    osPrefersDark(false);
    applyTheme("dark");
    expect(document.documentElement.classList.contains("dark")).toBe(true);
    expect(cachedTheme()).toBe("dark");
    applyTheme("system");
    expect(document.documentElement.classList.contains("dark")).toBe(false);
    expect(cachedTheme()).toBe("system");
  });

  it("cachedTheme defaults to dark for missing or invalid values", () => {
    expect(cachedTheme()).toBe("dark");
    localStorage.setItem("rolle-theme", "sepia");
    expect(cachedTheme()).toBe("dark");
    localStorage.setItem("rolle-theme", "light");
    expect(cachedTheme()).toBe("light");
  });

  it("watchSystemTheme re-applies only while the preference is system", () => {
    const os = osPrefersDark(false);
    let pref: "system" | "light" = "system";
    const stop = watchSystemTheme(() => pref);
    expect(os.listeners.size).toBe(1);
    os.fire();
    expect(document.documentElement.classList.contains("dark")).toBe(false);
    osPrefersDark(true);
    os.fire();
    expect(document.documentElement.classList.contains("dark")).toBe(true);
    pref = "light";
    osPrefersDark(false);
    os.fire();
    expect(document.documentElement.classList.contains("dark")).toBe(true);
    stop();
    expect(os.listeners.size).toBe(0);
  });
});
