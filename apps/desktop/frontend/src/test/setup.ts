import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { MotionGlobalConfig } from "motion/react";
import { afterEach, vi } from "vitest";

// Finish every animation at once so presence checks do not wait on timers.
MotionGlobalConfig.skipAnimations = true;

// Confetti draws on a canvas, which jsdom lacks. Stub the library so tests can observe celebrate().
vi.mock("canvas-confetti", () => ({ default: vi.fn(() => Promise.resolve()) }));

// The Wails runtime warns once per module graph that it runs outside the
// webview. Tests always do, so drop that one message and keep the rest.
const warn = console.warn.bind(console);
console.warn = (...args: unknown[]) => {
  if (typeof args[0] === "string" && args[0].includes("Browser Environment Detected")) return;
  warn(...args);
};

// jsdom has no matchMedia. Default to a light OS theme; tests override as needed.
function matchMedia(query: string): MediaQueryList {
  return {
    matches: false,
    media: query,
    onchange: null,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  } as unknown as MediaQueryList;
}
Object.defineProperty(window, "matchMedia", { writable: true, configurable: true, value: vi.fn(matchMedia) });

// cmdk and Radix use these; jsdom does not implement them.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver = globalThis.ResizeObserver ?? (ResizeObserverStub as unknown as typeof ResizeObserver);
Element.prototype.scrollIntoView = Element.prototype.scrollIntoView ?? vi.fn();
Element.prototype.hasPointerCapture = Element.prototype.hasPointerCapture ?? vi.fn(() => false);
Element.prototype.releasePointerCapture = Element.prototype.releasePointerCapture ?? vi.fn();

afterEach(() => {
  cleanup();
  localStorage.clear();
  vi.restoreAllMocks();
});
