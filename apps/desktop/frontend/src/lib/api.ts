import { useCallback, useEffect, useState } from "react";
import { Events } from "@wailsio/runtime";
import { RolleService } from "../../bindings/github.com/nateships/rolle/apps/desktop";
import type { Workspace } from "../../bindings/github.com/nateships/rolle/internal/core";

export { RolleService as api };
export type { Session, Integration, Credentials, Workspace } from "../../bindings/github.com/nateships/rolle/internal/core";
export { Kind, Status, Cloud } from "../../bindings/github.com/nateships/rolle/internal/core";

export const WORKSPACE_CHANGED = "workspace:changed";

/** Load the workspace and keep it fresh while the backend emits change events. */
export function useWorkspace() {
  const [workspace, setWorkspace] = useState<Workspace | null>(null);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    try {
      const w = await RolleService.Workspace();
      setWorkspace(w);
      setError(null);
    } catch (e) {
      setError(errorMessage(e));
    }
  }, []);

  useEffect(() => {
    void reload();
    const off = Events.On(WORKSPACE_CHANGED, () => void reload());
    return () => off();
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
