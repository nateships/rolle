import { afterEach, describe, expect, it, vi } from "vitest";

// copyText reads inWails at import time, so each test imports a fresh module.
async function load(inWails: boolean) {
  vi.resetModules();
  if (inWails) {
    vi.doMock("./api", () => ({ inWails: true }));
    vi.doMock("@wailsio/runtime", () => ({ Clipboard: { SetText: vi.fn(async () => {}) } }));
  }
  return import("./clipboard");
}

function stubBrowserClipboard(writeText: (text: string) => Promise<void>) {
  Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
}

describe("copyText", () => {
  afterEach(() => {
    vi.doUnmock("./api");
    vi.doUnmock("@wailsio/runtime");
  });

  it("writes to the browser clipboard outside Wails", async () => {
    const writeText = vi.fn(async () => {});
    stubBrowserClipboard(writeText);
    const { copyText } = await load(false);
    await copyText("hello");
    expect(writeText).toHaveBeenCalledWith("hello");
  });

  it("passes a browser clipboard failure to the caller", async () => {
    stubBrowserClipboard(async () => {
      throw new Error("denied");
    });
    const { copyText } = await load(false);
    await expect(copyText("x")).rejects.toThrow("denied");
  });

  it("uses the Wails clipboard inside the app", async () => {
    const writeText = vi.fn(async () => {});
    stubBrowserClipboard(writeText);
    const { copyText } = await load(true);
    const { Clipboard } = await import("@wailsio/runtime");
    await copyText("inside");
    expect(Clipboard.SetText).toHaveBeenCalledWith("inside");
    expect(writeText).not.toHaveBeenCalled();
  });
});
