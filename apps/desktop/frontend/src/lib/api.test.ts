import { renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { api, errorMessage, inWails, useWorkspace } from "@/lib/api";

describe("api", () => {
  it("uses the browser mock outside Wails", () => {
    expect(inWails).toBe(false);
    expect(typeof api.Workspace).toBe("function");
  });

  it("useWorkspace normalises nil Go slices to empty arrays", async () => {
    vi.spyOn(api, "Workspace").mockResolvedValue({
      version: 1,
      onboarded: false,
      sessions: null,
      integrations: null,
    } as never);
    const { result } = renderHook(() => useWorkspace());
    await waitFor(() => expect(result.current.workspace).not.toBeNull());
    expect(result.current.workspace?.sessions).toEqual([]);
    expect(result.current.workspace?.integrations).toEqual([]);
    expect(result.current.error).toBeNull();
  });

  it("useWorkspace reports a load failure", async () => {
    vi.spyOn(api, "Workspace").mockRejectedValue(new Error("boom"));
    const { result } = renderHook(() => useWorkspace());
    await waitFor(() => expect(result.current.error).toBe("boom"));
    expect(result.current.workspace).toBeNull();
  });
});

describe("errorMessage", () => {
  it("reads Errors, strings and objects", () => {
    expect(errorMessage(new Error("x"))).toBe("x");
    expect(errorMessage("plain")).toBe("plain");
    expect(errorMessage({ code: 1 })).toBe('{"code":1}');
  });

  it("falls back when the value cannot be serialised", () => {
    const cyclic: Record<string, unknown> = {};
    cyclic.self = cyclic;
    expect(errorMessage(cyclic)).toBe("Unknown error");
  });
});
