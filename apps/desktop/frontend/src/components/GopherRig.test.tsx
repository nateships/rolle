import { act, fireEvent, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { GopherRig } from "@/components/GopherRig";

describe("GopherRig", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  function part(container: HTMLElement, name: string) {
    return container.querySelector(`[data-part="${name}"]`)!;
  }

  it("plays the named move on its own with autoplay", () => {
    // Random would pick the peek; the named move wins.
    vi.spyOn(Math, "random").mockReturnValue(0.9);
    const { container } = render(<GopherRig autoplay="dance" />);
    // Nothing moves before the section has settled.
    act(() => void vi.advanceTimersByTime(200));
    expect(part(container, "hands-root").getAttribute("transform")).toBeNull();
    act(() => void vi.advanceTimersByTime(500));
    expect(part(container, "hands-root").getAttribute("transform")).not.toBeNull();
  });

  it("picks a random move on its own with plain autoplay", () => {
    vi.spyOn(Math, "random").mockReturnValue(0);
    const { container } = render(<GopherRig autoplay />);
    act(() => void vi.advanceTimersByTime(700));
    expect(part(container, "hands-root").getAttribute("transform")).not.toBeNull();
  });

  it("returns every part to rest after the dance and after the peek", () => {
    const { container } = render(<GopherRig />);
    const svg = container.querySelector("svg")!;
    const random = vi.spyOn(Math, "random");
    // Dance.
    random.mockReturnValue(0);
    fireEvent.click(svg);
    act(() => void vi.advanceTimersByTime(300));
    expect(part(container, "hands-root").getAttribute("transform")).not.toBeNull();
    act(() => void vi.advanceTimersByTime(3000));
    for (const name of ["gopher-root", "hands-root", "pupil-left", "pupil-right"]) {
      expect(part(container, name).getAttribute("transform"), name).toBeNull();
    }
    // Hide and peek.
    random.mockReturnValue(0.9);
    fireEvent.click(svg);
    act(() => void vi.advanceTimersByTime(300));
    expect(part(container, "gopher-root").getAttribute("transform")).toMatch(/translate/);
    act(() => void vi.advanceTimersByTime(3000));
    expect(part(container, "gopher-root").getAttribute("transform")).toBeNull();
    expect(part(container, "hands-root").getAttribute("transform")).toBeNull();
    expect(part(container, "hands-root").getAttribute("opacity")).toBeNull();
  });
});
