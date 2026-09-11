import confetti from "canvas-confetti";

/** Fire a two-sided burst. Safe to call repeatedly. */
export function celebrate(intensity: "small" | "big" = "big") {
  const count = intensity === "big" ? 160 : 60;
  const defaults = { spread: 70, ticks: 200, gravity: 0.9, scalar: 0.9, zIndex: 9999, disableForReducedMotion: true };
  const colors = ["#f59e0b", "#38bdf8", "#34d399", "#a78bfa", "#f472b6", "#f8fafc"];
  void confetti({ ...defaults, particleCount: count, angle: 60, origin: { x: 0, y: 0.7 }, colors });
  void confetti({ ...defaults, particleCount: count, angle: 120, origin: { x: 1, y: 0.7 }, colors });
  if (intensity === "big") {
    setTimeout(
      () =>
        void confetti({
          ...defaults,
          particleCount: 90,
          spread: 120,
          startVelocity: 35,
          origin: { x: 0.5, y: 0.4 },
          colors,
        }),
      250,
    );
  }
}
