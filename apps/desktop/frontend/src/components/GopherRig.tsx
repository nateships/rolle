import { useId, useRef } from "react";
import { cn } from "@/lib/utils";

/**
 * The gopher on its cards as editable vector layers, from docs/brand/animation-kit.
 * A click alternates between a short dance and a hide-then-peek behind the green
 * card. Nothing else happens; that is the point. Reduced motion skips it.
 */
export function GopherRig({ className }: { className?: string }) {
  const uid = useId().replace(/:/g, "");
  const svg = useRef<SVGSVGElement>(null);
  const state = useRef<{ frame: number; busy: boolean; next: "dance" | "peek" }>({
    frame: 0,
    busy: false,
    next: "dance",
  });

  function part(name: string): SVGGElement | null {
    return svg.current?.querySelector(`[data-part="${name}"]`) ?? null;
  }
  const tr = (name: string, value: string) => part(name)?.setAttribute("transform", value);

  function reset() {
    for (const name of ["gopher-root", "hands-root", "pupil-left", "pupil-right"]) {
      part(name)?.removeAttribute("transform");
    }
    part("hands-root")?.removeAttribute("opacity");
  }

  function animate(ms: number, paint: (t: number) => void, done?: () => void) {
    const begin = performance.now();
    const tick = (now: number) => {
      const t = Math.min(1, (now - begin) / ms);
      paint(t);
      if (t < 1) state.current.frame = requestAnimationFrame(tick);
      else done?.();
    };
    state.current.frame = requestAnimationFrame(tick);
  }

  const ease = (t: number) => 1 - Math.pow(1 - t, 3);

  // Bounce and sway in place, arms and feet on their pivots, pupils along.
  function dance(t: number) {
    const beat = t * 8 * Math.PI;
    const swing = Math.sin(beat);
    const body = `translate(${swing * 6} ${-Math.abs(swing) * 14}) rotate(${swing * 6} 246 270)`;
    tr("gopher-root", body);
    tr("hands-root", body);
    tr("arm-left", `rotate(${swing * 24 + 25} 158 170)`);
    tr("arm-right", `rotate(${swing * 24 - 25} 334 170)`);
    tr("hand-left", `rotate(${swing * 24 + 25} 158 170) translate(-28 44)`);
    tr("hand-right", `rotate(${swing * 24 - 25} 334 170) translate(18 45)`);
    tr("foot-left", `rotate(${swing * 20} 195 263)`);
    tr("foot-right", `rotate(${-swing * 20} 301 263)`);
    tr("pupil-left", `translate(${swing * 3} 0)`);
    tr("pupil-right", `translate(${swing * 3} 0)`);
  }

  // Paws let go, the gopher drops behind the green card, waits, then climbs back.
  function pose(dy: number) {
    tr("gopher-root", `translate(0 ${dy})`);
    tr("hands-root", `translate(0 ${dy})`);
  }
  function hideThenPeek() {
    const hands = part("hands-root");
    animate(
      650,
      (t) => {
        pose(ease(t) * 370);
        hands?.setAttribute("opacity", String(Math.max(0, 1 - t * 4)));
      },
      () =>
        setTimeout(
          () =>
            animate(
              900,
              (t) => {
                pose((1 - ease(t)) * 370);
                hands?.setAttribute("opacity", String(Math.max(0, (t - 0.7) / 0.3)));
              },
              finish,
            ),
          450,
        ),
    );
  }

  function finish() {
    reset();
    state.current.busy = false;
  }

  function play() {
    if (state.current.busy) return;
    if (typeof matchMedia === "function" && matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    state.current.busy = true;
    reset();
    if (state.current.next === "dance") {
      state.current.next = "peek";
      animate(2400, dance, finish);
    } else {
      state.current.next = "dance";
      hideThenPeek();
    }
  }

  return (
    <svg ref={svg} viewBox="0 0 385 310" className={cn("size-full", className)} aria-hidden onClick={play}>
      <defs>
        <mask id={`${uid}-front`} maskUnits="userSpaceOnUse" x="-200" y="-200" width="1000" height="1000">
          <rect x="-200" y="-200" width="1000" height="1000" fill="white" />
          <path
            d="M139 127 H157 Q249 148 340 127 H346 Q379 127 379 160 V255 Q379 287 346 287 H140 Q108 287 108 256 V161 Q108 127 139 127 Z"
            fill="black"
          />
          <rect x="-200" y="287" width="1000" height="800" fill="black" />
        </mask>
        <mask id={`${uid}-middle`} maskUnits="userSpaceOnUse" x="-200" y="-200" width="1000" height="1000">
          <rect x="-200" y="-200" width="1000" height="1000" fill="white" />
          <path
            d="M89 86 H295 Q328 86 328 119 V225 Q328 259 295 259 H98 L80 255 Q57 250 57 225 V119 Q57 86 89 86 Z"
            fill="black"
          />
          <path
            d="M139 127 H157 Q249 148 340 127 H346 Q379 127 379 160 V255 Q379 287 346 287 H140 Q108 287 108 256 V161 Q108 127 139 127 Z"
            fill="black"
          />
          <rect x="-200" y="259" width="1000" height="800" fill="black" />
        </mask>
        <mask id={`${uid}-back`} maskUnits="userSpaceOnUse" x="-200" y="-200" width="1000" height="1000">
          <rect x="-200" y="-200" width="1000" height="1000" fill="white" />
          <path
            d="M40 45 H247 Q277 45 277 77 V189 Q277 220 246 220 H49 L32 216 Q7 211 7 186 V78 Q7 45 40 45 Z"
            fill="black"
          />
          <path
            d="M89 86 H295 Q328 86 328 119 V225 Q328 259 295 259 H98 L80 255 Q57 250 57 225 V119 Q57 86 89 86 Z"
            fill="black"
          />
          <path
            d="M139 127 H157 Q249 148 340 127 H346 Q379 127 379 160 V255 Q379 287 346 287 H140 Q108 287 108 256 V161 Q108 127 139 127 Z"
            fill="black"
          />
          <rect x="-200" y="220" width="1000" height="800" fill="black" />
        </mask>
      </defs>
      <g data-part="scene">
        <g data-part="card-blue">
          <path
            fill="#244CFF"
            d="M40 45 H235 Q252 45 256 62 L260 74 H81 Q45 74 45 112 V220 L32 216 Q7 211 7 186 V78 Q7 45 40 45 Z"
          />
        </g>
        <g data-part="card-orange">
          <path
            fill="#FF7900"
            d="M89 86 H287 Q304 86 308 103 L312 115 H131 Q95 115 95 154 V259 L80 255 Q57 250 57 225 V119 Q57 86 89 86 Z"
          />
        </g>
        <g data-part="gopher-depth" mask={`url(#${uid}-front)`}>
          <g data-part="gopher-root">
            <g
              data-part="ear-left"
              fill="#F4F0E8"
              stroke="#101114"
              strokeWidth="4"
              strokeLinejoin="round"
              strokeLinecap="round"
            >
              <path d="M165 32 C145 17 124 31 127 51 C129 66 140 72 151 73 Z" />
              <ellipse cx="151" cy="51" rx="10" ry="10" fill="#101114" stroke="none" />
            </g>
            <g
              data-part="ear-right"
              fill="#F4F0E8"
              stroke="#101114"
              strokeWidth="4"
              strokeLinejoin="round"
              strokeLinecap="round"
            >
              <path d="M325 32 C346 14 368 30 364 50 C362 64 351 70 339 72 Z" />
              <ellipse cx="341" cy="49" rx="10" ry="10" fill="#101114" stroke="none" />
            </g>
            <g
              data-part="foot-left"
              fill="#F4F0E8"
              stroke="#101114"
              strokeWidth="4"
              strokeLinejoin="round"
              strokeLinecap="round"
            >
              <path d="M180 250 Q174 268 171 276 Q173 283 189 282 L210 280 Q217 276 210 259 Z" />
            </g>
            <g
              data-part="foot-right"
              fill="#F4F0E8"
              stroke="#101114"
              strokeWidth="4"
              strokeLinejoin="round"
              strokeLinecap="round"
            >
              <path d="M284 259 Q279 276 286 280 L309 282 Q326 283 326 275 L316 250 Z" />
            </g>
            <g
              data-part="arm-left"
              fill="#F4F0E8"
              stroke="#101114"
              strokeWidth="4"
              strokeLinejoin="round"
              strokeLinecap="round"
            >
              <path d="M158 158 Q134 151 132 174 Q132 194 153 194 L168 184 Z" />
            </g>
            <g
              data-part="arm-right"
              fill="#F4F0E8"
              stroke="#101114"
              strokeWidth="4"
              strokeLinejoin="round"
              strokeLinecap="round"
            >
              <path d="M332 158 Q356 151 358 174 Q358 194 337 194 L322 184 Z" />
            </g>
            <g
              data-part="body"
              fill="#F4F0E8"
              stroke="#101114"
              strokeWidth="4"
              strokeLinejoin="round"
              strokeLinecap="round"
            >
              <path d="M145 124 C145 44 178 7 240 7 C315 3 345 52 345 122 L345 213 C345 257 326 273 299 274 L192 274 C158 273 145 253 145 214 Z" />
            </g>
            <g
              data-part="eye-left"
              fill="#F4F0E8"
              stroke="#101114"
              strokeWidth="4"
              strokeLinejoin="round"
              strokeLinecap="round"
            >
              <ellipse cx="207" cy="61" rx="32" ry="32" strokeWidth="3.5" />
            </g>
            <g
              data-part="pupil-left"
              fill="#F4F0E8"
              stroke="#101114"
              strokeWidth="4"
              strokeLinejoin="round"
              strokeLinecap="round"
            >
              <ellipse cx="193" cy="64" rx="11" ry="12" fill="#101114" stroke="none" />
            </g>
            <g
              data-part="eye-right"
              fill="#F4F0E8"
              stroke="#101114"
              strokeWidth="4"
              strokeLinejoin="round"
              strokeLinecap="round"
            >
              <ellipse cx="287" cy="60" rx="32" ry="32" strokeWidth="3.5" />
            </g>
            <g
              data-part="pupil-right"
              fill="#F4F0E8"
              stroke="#101114"
              strokeWidth="4"
              strokeLinejoin="round"
              strokeLinecap="round"
            >
              <ellipse cx="273" cy="63" rx="11" ry="12" fill="#101114" stroke="none" />
            </g>
            <g
              data-part="teeth"
              fill="#F4F0E8"
              stroke="#101114"
              strokeWidth="4"
              strokeLinejoin="round"
              strokeLinecap="round"
            >
              <path d="M233 107 L233 128 Q233 135 241 135 Q249 135 249 128 L249 107 Z" strokeWidth="3.5" />
              <path d="M249 107 L249 128 Q249 135 257 135 Q265 135 265 128 L265 107 Z" strokeWidth="3.5" />
            </g>
            <g
              data-part="muzzle"
              fill="#F4F0E8"
              stroke="#101114"
              strokeWidth="4"
              strokeLinejoin="round"
              strokeLinecap="round"
            >
              <path
                d="M246 92 C228 93 219 104 224 111 C228 118 240 111 249 110 C258 111 269 118 273 111 C278 102 266 93 253 92 Z"
                strokeWidth="3.5"
              />
            </g>
            {/* The card hides the body's own outline at the chin. This line, two units above the card edge, keeps the dark seam the flat artwork had. */}
            <g data-part="chin" fill="none" stroke="#101114" strokeWidth="4" strokeLinecap="round">
              <path d="M150 125 Q249 146 348 125" />
            </g>
            <g
              data-part="nose"
              fill="#F4F0E8"
              stroke="#101114"
              strokeWidth="4"
              strokeLinejoin="round"
              strokeLinecap="round"
            >
              <ellipse cx="249" cy="90" rx="14" ry="9" fill="#101114" stroke="none" />
            </g>
          </g>
        </g>
        <g data-part="card-green">
          <path
            fill="#00CE78"
            d="M139 127 H157 Q249 148 340 127 H346 Q379 127 379 160 V255 Q379 287 346 287 H238 V235 Q238 211 262 211 H286 Q290 211 290 207 V181 Q290 176 286 176 H251 Q187 176 187 241 V287 H140 Q108 287 108 256 V161 Q108 127 139 127 Z"
          />
        </g>
        <g data-part="hands-root">
          <g
            data-part="hand-left"
            fill="#F4F0E8"
            stroke="#101114"
            strokeWidth="4"
            strokeLinejoin="round"
            strokeLinecap="round"
          >
            <circle cx="168" cy="135" r="20" strokeWidth="7" />
          </g>
          <g
            data-part="hand-right"
            fill="#F4F0E8"
            stroke="#101114"
            strokeWidth="4"
            strokeLinejoin="round"
            strokeLinecap="round"
          >
            <circle cx="333" cy="134" r="20" strokeWidth="7" />
          </g>
        </g>
      </g>
    </svg>
  );
}
