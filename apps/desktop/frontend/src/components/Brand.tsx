import { motion } from "motion/react";
import { cn } from "@/lib/utils";

/** Rolle mark: three concentric rings, each a role you can step into. */
export function Mark({ className, animate = false }: { className?: string; animate?: boolean }) {
  const rings = [
    { r: 40, dash: "60 40", dur: 14 },
    { r: 28, dash: "30 25", dur: 9 },
    { r: 16, dash: "18 14", dur: 6 },
  ];
  return (
    <svg viewBox="0 0 100 100" className={cn("size-10", className)} aria-hidden>
      {rings.map((ring, i) => (
        <motion.circle
          key={ring.r}
          cx="50"
          cy="50"
          r={ring.r}
          fill="none"
          stroke="currentColor"
          strokeWidth={i === 2 ? 6 : 4}
          strokeLinecap="round"
          strokeDasharray={ring.dash}
          style={{ originX: "50px", originY: "50px", opacity: 1 - i * 0.25 }}
          animate={animate ? { rotate: i % 2 === 0 ? 360 : -360 } : undefined}
          transition={animate ? { repeat: Infinity, ease: "linear", duration: ring.dur } : undefined}
        />
      ))}
      <circle cx="50" cy="50" r="4" fill="currentColor" />
    </svg>
  );
}

export function CloudGlyph({ cloud, className }: { cloud: "aws" | "azure" | "gcp"; className?: string }) {
  const label = { aws: "AWS", azure: "AZ", gcp: "GCP" }[cloud];
  const tone = {
    aws: "bg-orange-500/15 text-orange-300 ring-orange-500/30",
    azure: "bg-sky-500/15 text-sky-300 ring-sky-500/30",
    gcp: "bg-emerald-500/15 text-emerald-300 ring-emerald-500/30",
  }[cloud];
  return (
    <span className={cn("inline-flex size-8 shrink-0 items-center justify-center rounded-md text-[10px] font-bold tracking-wide ring-1", tone, className)}>
      {label}
    </span>
  );
}
