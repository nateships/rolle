import { useCallback, useEffect, useRef, useState } from "react";
import { Events } from "@wailsio/runtime";
import { RolleService } from "../../bindings/github.com/nateships/rolle/apps/desktop";
import type { Integration, Session, Workspace as CoreWorkspace } from "../../bindings/github.com/nateships/rolle/internal/core";

/** Workspace with the nullable Go slices normalised to arrays. */
export type Workspace = Omit<CoreWorkspace, "sessions" | "integrations"> & { sessions: Session[]; integrations: Integration[] };

function normalise(w: CoreWorkspace | null): Workspace | null {
  if (!w) return null;
  return { ...w, sessions: w.sessions ?? [], integrations: w.integrations ?? [] };
}
import { mockApi } from "./mock";

/** True when running inside the Wails webview rather than a plain browser.
 *  Mirrors the runtime's own transport detection: WebView2, WKWebView, or Android. */
export const inWails = (() => {
  if (typeof window === "undefined") return false;
  const w = window as unknown as { chrome?: { webview?: { postMessage?: unknown } }; webkit?: { messageHandlers?: { external?: { postMessage?: unknown } } }; wails?: { invoke?: unknown } };
  return !!(w.chrome?.webview?.postMessage || w.webkit?.messageHandlers?.external?.postMessage || w.wails?.invoke);
})();

// Outside Wails (plain `aube run dev`) fall back to an in-memory mock so the UI
// can be designed and demoed without the Go backend.
export const api: typeof RolleService = inWails ? RolleService : (mockApi as unknown as typeof RolleService);
export type { Session, Integration, Credentials, Settings } from "../../bindings/github.com/nateships/rolle/internal/core";
export type { AppInfo, DeviceLogin, GCPStatus, UpdateInfo } from "../../bindings/github.com/nateships/rolle/apps/desktop";
export { Kind, Status, Cloud } from "../../bindings/github.com/nateships/rolle/internal/core";

export const WORKSPACE_CHANGED = "workspace:changed";

/** Load the workspace and keep it fresh while the backend emits change events. */
export function useWorkspace() {
  const [workspace, setWorkspace] = useState<Workspace | null>(null);
  const [error, setError] = useState<string | null>(null);
  // Only the newest request may update state; a slow older one is stale.
  const seq = useRef(0);

  const reload = useCallback(async () => {
    const mine = ++seq.current;
    try {
      const w = await api.Workspace();
      if (mine !== seq.current) return;
      setWorkspace(normalise(w));
      setError(null);
    } catch (e) {
      if (mine === seq.current) setError(errorMessage(e));
    }
  }, []);

  useEffect(() => {
    void reload();
    const off = inWails ? Events.On(WORKSPACE_CHANGED, () => void reload()) : mockApi.onChange(() => void reload());
    return () => void off();
  }, [reload]);

  return { workspace, error, reload };
}

export function errorMessage(e: unknown): string {
  if (e instanceof Error) return e.message;
  if (typeof e === "string") return e;
  try {
    return JSON.stringify(e);
  } catch {
    return "Unknown error";
  }
}
