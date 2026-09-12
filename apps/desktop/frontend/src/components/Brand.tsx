import { motion, useAnimation } from "motion/react";
import { cn } from "@/lib/utils";
import gopherCards from "@/assets/brand/gopher-cards.png";
import gopherPeek from "@/assets/brand/gopher-peek.png";
import wordmarkCharcoal from "@/assets/brand/wordmark-charcoal.svg";
import wordmarkIvory from "@/assets/brand/wordmark-ivory.svg";
import awsLogoDark from "@/assets/vendors/aws-dark.svg";
import awsLogoLight from "@/assets/vendors/aws-light.svg";
import azureLogo from "@/assets/vendors/azure.svg";
import gcpLogo from "@/assets/vendors/gcp.svg";

/**
 * The gopher artwork at full size. On light surfaces a blurred gray halo sits behind it so the
 * ivory body stays visible. The halo scales with the artwork and fades into the background.
 */
export function GopherMark({ className }: { className?: string }) {
  const controls = useAnimation();
  // Click the gopher and it dances on the cards. Nothing else happens; that is the point.
  const dance = () =>
    void controls.start({
      y: [0, -12, 0, -12, 0, -7, 0],
      x: [0, -4, 0, 4, 0, -2, 0],
      rotate: [0, -9, 0, 9, 0, -5, 0],
      transition: { duration: 1.3, ease: "easeInOut" },
    });
  return (
    <span
      className={cn(
        "no-drag @container relative isolate inline-block aspect-[385/294] h-16 shrink-0 select-none before:absolute before:inset-[6%] before:-z-10 before:rounded-[20%] before:bg-[#8b9099] before:blur-[9cqw] dark:before:hidden",
        className,
      )}
      onClick={dance}
    >
      {/* Two layers from tools/gopherlayers: the cards, and the gopher on top of them. */}
      <img
        src={gopherCards}
        alt=""
        aria-hidden
        draggable={false}
        className="absolute inset-0 size-full object-contain"
      />
      <motion.img
        src={gopherPeek}
        alt=""
        aria-hidden
        draggable={false}
        animate={controls}
        style={{ originX: 0.5, originY: 0.85 }}
        className="absolute inset-0 size-full object-contain"
      />
    </span>
  );
}

/** The gopher with the wordmark beside it. Height comes from className; the parts scale with it. */
export function GopherLockup({ className }: { className?: string }) {
  return (
    <span className={cn("inline-flex h-10 items-center gap-[0.15em]", className)}>
      <GopherMark className="h-full w-auto" />
      <img src={wordmarkCharcoal} alt="Rolle" className="h-[55%] w-auto select-none dark:hidden" draggable={false} />
      <img src={wordmarkIvory} alt="Rolle" className="hidden h-[55%] w-auto select-none dark:block" draggable={false} />
    </span>
  );
}

// AWS ships dark text, so it needs a light-text copy for dark surfaces.
const VENDORS = {
  aws: { src: awsLogoLight, dark: awsLogoDark, name: "Amazon Web Services" },
  azure: { src: azureLogo, name: "Microsoft Azure" },
  gcp: { src: gcpLogo, name: "Google Cloud" },
} as const;

/** Provider mark on a neutral tile. The logo identifies the vendor; state is never encoded here. */
export function CloudGlyph({ cloud, className }: { cloud: "aws" | "azure" | "gcp"; className?: string }) {
  const v = VENDORS[cloud];
  return (
    <span
      title={v.name}
      className={cn(
        "inline-flex size-8 shrink-0 items-center justify-center rounded-md border border-border bg-card p-1.5",
        className,
      )}
    >
      <img
        src={v.src}
        alt={v.name}
        className={cn("size-full object-contain", "dark" in v && "dark:hidden")}
        draggable={false}
      />
      {"dark" in v && (
        <img src={v.dark} alt="" aria-hidden className="hidden size-full object-contain dark:block" draggable={false} />
      )}
    </span>
  );
}
