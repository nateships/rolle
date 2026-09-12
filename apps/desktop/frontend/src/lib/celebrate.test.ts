import confetti from "canvas-confetti";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { celebrate } from "@/lib/celebrate";

// The test setup replaces canvas-confetti with a mock.
const fire = vi.mocked(confetti);

describe("celebrate", () => {
  beforeEach(() => {
    fire.mockClear();
    vi.useFakeTimers();
  });
  afterEach(() => vi.useRealTimers());

  it("fires one burst from each side for a small celebration", () => {
    celebrate("small");
    expect(fire).toHaveBeenCalledTimes(2);
    const [left, right] = fire.mock.calls.map((c) => c[0]);
    expect(left).toMatchObject({ particleCount: 60, angle: 60, origin: { x: 0, y: 0.7 } });
    expect(right).toMatchObject({ particleCount: 60, angle: 120, origin: { x: 1, y: 0.7 } });
    // A small celebration has no delayed burst.
    vi.runAllTimers();
    expect(fire).toHaveBeenCalledTimes(2);
  });

  it("adds a delayed centre burst for a big celebration", () => {
    celebrate();
    expect(fire).toHaveBeenCalledTimes(2);
    expect(fire.mock.calls[0][0]).toMatchObject({ particleCount: 160 });
    vi.advanceTimersByTime(249);
    expect(fire).toHaveBeenCalledTimes(2);
    vi.advanceTimersByTime(1);
    expect(fire).toHaveBeenCalledTimes(3);
    expect(fire.mock.calls[2][0]).toMatchObject({ particleCount: 90, spread: 120, origin: { x: 0.5, y: 0.4 } });
  });

  it("asks the library to respect reduced motion", () => {
    celebrate();
    vi.runAllTimers();
    for (const [opts] of fire.mock.calls) expect(opts).toMatchObject({ disableForReducedMotion: true });
  });
});
