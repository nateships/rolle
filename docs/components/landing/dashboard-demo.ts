import { sessions } from "./demo-sessions";

export function initDashboardDemo() {
  const motionPreference = matchMedia("(prefers-reduced-motion: reduce)");
  let reduced = motionPreference.matches;
  const fine = matchMedia("(hover: hover) and (pointer: fine)").matches;

  // ---- The miniature app -------------------------------------------------
  const stage = document.getElementById("stage")!;
  const app = stage.querySelector<HTMLElement>("#app")!;
  const ghost = stage.querySelector<HTMLElement>("#ghost")!;
  const term = stage.querySelector<HTMLElement>("#term")!;
  const termOut = stage.querySelector<HTMLElement>("#term-out")!;
  const rows = [...app.querySelectorAll<HTMLElement>(".row[data-id]")];
  const byId = (id: string) => rows.find((r) => r.dataset.id === id)!;
  const secs = new Map<string, number>();
  const fmt = (s: number) => `${Math.floor(s / 60)}m ${String(s % 60).padStart(2, "0")}s`;
  const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

  const paintState = (row: HTMLElement) => {
    if (row.classList.contains("group")) return;
    const st = row.querySelector(".state")!;
    const s = secs.get(row.dataset.id!);
    if (s === undefined) {
      row.classList.remove("active", "expired");
      st.textContent = "Inactive";
    } else if (s <= 0) {
      row.classList.add("active", "expired");
      st.innerHTML = "<b>Expired</b><span class=clock>renewing</span>";
    } else {
      row.classList.add("active");
      row.classList.remove("expired");
      st.innerHTML = `<b>Active</b><span class=clock>${fmt(s)}</span>`;
    }
    const n = [...secs.values()].filter((v) => v > 0).length;
    stage.querySelector("#side-active")!.textContent = String(n);
    stage.querySelector("#foot-active")!.textContent = `${n} active`;
  };

  const start = (id: string, s = 3599) => {
    secs.set(id, s);
    paintState(byId(id));
  };
  const stop = (id: string) => {
    secs.delete(id);
    paintState(byId(id));
  };

  rows.forEach((row) => {
    const id = row.dataset.id!;
    if (row.dataset.secs) secs.set(id, Number(row.dataset.secs));
    paintState(row);
  });

  // Time passes. A session that runs out renews a few seconds later.
  setInterval(() => {
    if (document.hidden) return;
    for (const [id, s] of secs) {
      const next = s - 1;
      secs.set(id, next <= -4 ? 3599 : next);
      paintState(byId(id));
    }
  }, 1000);

  // Terminal lines come from the demo data, not from the DOM, and render as
  // text nodes. Nothing user-visible is ever parsed as HTML.
  type Line = { cls: "p" | "o"; text: string };
  const termFor = (id: string): { command: Line[]; output: Line[] } => {
    const session = sessions.find((session) => session.id === id)!;
    const prompt: Line = { cls: "p", text: "$ " };
    if (session.profile) {
      return {
        command: [prompt, { cls: "o", text: `aws sts get-caller-identity --profile ${session.profile}` }],
        output: [
          {
            cls: "o",
            text: `{ "Account": "123456789012",\n  "Arn": "arn:aws:sts::123456789012:assumed-role/${session.name}/nate" }`,
          },
        ],
      };
    }
    if (session.cloud === "azure") {
      return {
        command: [prompt, { cls: "o", text: "az account show --query name" }],
        output: [{ cls: "o", text: `"${session.name}"` }],
      };
    }
    return {
      command: [prompt, { cls: "o", text: "gcloud config get project" }],
      output: [{ cls: "o", text: `${session.name}-4821` }],
    };
  };
  const span = (cls: string, text: string) => {
    const el = document.createElement("span");
    el.className = cls;
    el.textContent = text;
    return el;
  };
  const renderTerm = (lines: { command: Line[]; output: Line[] }, typed?: number, cursor = false) => {
    termOut.replaceChildren();
    for (const [i, line] of lines.command.entries()) {
      const text = typed !== undefined && i === lines.command.length - 1 ? line.text.slice(0, typed) : line.text;
      termOut.append(span(line.cls, text));
    }
    if (cursor) termOut.append(span("cur", "▍"));
    if (typed === undefined) {
      termOut.append(document.createTextNode("\n"));
      for (const line of lines.output) termOut.append(span(line.cls, line.text));
    }
  };

  let typing = 0;
  let terminalId: string | undefined;
  const openTerm = async (id: string) => {
    const me = ++typing;
    terminalId = id;
    const lines = termFor(id);
    const command = lines.command[lines.command.length - 1].text;
    term.classList.add("open");
    if (reduced) {
      renderTerm(lines);
      return;
    }
    renderTerm(lines, 0, true);
    await sleep(350);
    for (let i = 1; i <= command.length && me === typing; i++) {
      renderTerm(lines, i, true);
      await sleep(22);
    }
    if (me !== typing) return;
    await sleep(300);
    if (me !== typing) return;
    renderTerm(lines);
  };
  term.querySelector(".x")!.addEventListener("click", () => {
    ++typing;
    term.classList.remove("open");
  });

  // Real clicks do real things.
  app.addEventListener("click", (e) => {
    const t = e.target as HTMLElement;
    const row = t.closest<HTMLElement>(".row[data-id]");
    if (!row) return;
    const id = row.dataset.id!;
    if (t.closest(".play")) {
      secs.has(id) ? stop(id) : start(id);
    } else if (t.closest("[data-act=term]")) {
      void openTerm(id);
    } else if (t.closest("[data-act=copy]")) {
      const b = t.closest("button")!;
      b.style.color = "var(--a-green)";
      setTimeout(() => (b.style.color = ""), 800);
    }
  });

  // ---- The ghost runs the workflow until you take over ------------------
  let run = 0;
  let takenOver = false;
  let initialDemo: ReturnType<typeof setTimeout> | undefined;
  const toast = stage.querySelector<HTMLElement>("#toast")!;
  let toastTimer: ReturnType<typeof setTimeout> | undefined;
  const takeOver = () => {
    if (takenOver) return;
    takenOver = true;
    clearTimeout(initialDemo);
    clearTimeout(toastTimer);
    ++run;
    ++typing;
    ghost.style.opacity = "0";
    ghost.classList.remove("click");
    toast.classList.remove("show");
    app.querySelectorAll(".press, .pulse, .flash").forEach((el) => el.classList.remove("press", "pulse", "flash"));
  };
  // Capture takeover before the control's own action runs, including
  // keyboard activation. The ghost never dispatches these input events.
  stage.addEventListener("pointerdown", takeOver, { capture: true });
  stage.addEventListener("keydown", takeOver, { capture: true });
  stage.addEventListener("click", takeOver, { capture: true });
  const say = (html: string, warn = false) => {
    clearTimeout(toastTimer);
    toast.innerHTML = html;
    toast.className = warn ? "toast show warn" : "toast show";
    toastTimer = setTimeout(() => toast.classList.remove("show"), 2600);
  };
  const pulse = (id: string, cls = "pulse") => {
    const row = byId(id);
    row.classList.remove(cls);
    void row.offsetWidth;
    row.classList.add(cls);
  };

  const moveTo = async (el: Element, dx = 0.5, dy = 0.5) => {
    const a = app.getBoundingClientRect();
    const b = el.getBoundingClientRect();
    ghost.style.setProperty("--gx", `${b.left - a.left + b.width * dx}px`);
    ghost.style.setProperty("--gy", `${b.top - a.top + b.height * dy}px`);
    await sleep(reduced ? 0 : 800);
  };
  const click = async (el: Element) => {
    ghost.classList.remove("click");
    void ghost.offsetWidth;
    ghost.classList.add("click");
    el.classList.add("press");
    await sleep(160);
    el.classList.remove("press");
  };

  const reset = () => {
    for (const id of [...secs.keys()]) secs.delete(id);
    start("admin", 2814);
    term.classList.remove("open");
  };

  const demo = async () => {
    if (takenOver || reduced) return;
    const me = ++run;
    const alive = () => !takenOver && !reduced && me === run;
    reset();
    await moveTo(byId("readonly"), 0.35, 0.5);
    if (!alive()) return;
    await sleep(600);
    if (!alive()) return;
    const play = byId("prod-admin").querySelector(".play")!;
    await moveTo(play);
    if (!alive()) return;
    await click(play);
    if (!alive()) return;
    start("prod-admin", 3599);
    pulse("prod-admin");
    say("<b>Started</b> prod-admin. Credentials ready for the AWS CLI and SDKs.");
    await sleep(1600);
    if (!alive()) return;
    const t = byId("prod-admin").querySelector("[data-act=term]")!;
    await moveTo(t);
    if (!alive()) return;
    await click(t);
    if (!alive()) return;
    await openTerm("prod-admin");
    if (!alive()) return;
    await sleep(3200);
    if (!alive()) return;
    const cplay = byId("contoso").querySelector(".play")!;
    await moveTo(cplay);
    if (!alive()) return;
    await click(cplay);
    if (!alive()) return;
    start("contoso", 3599);
    pulse("contoso");
    say("<b>Started</b> Contoso Production. Azure tokens exported to your shell.");
    term.classList.remove("open");
    await sleep(1400);
    if (!alive()) return;
    // Let the first session run out and renew while the pointer rests.
    secs.set("admin", 4);
    await moveTo(byId("data-platform"), 0.9, 0.5);
    if (!alive()) return;
    await sleep(4200);
    if (!alive()) return;
    pulse("admin", "flash");
    say("<b>Expired</b> AdministratorAccess. Renewing from Identity Center.", true);
    await sleep(3000);
    if (!alive()) return;
    pulse("admin");
    say("<b>Renewed</b> AdministratorAccess. Another hour, nothing written to disk.");
    await sleep(1800);
    if (!alive()) return;
    const splay = byId("prod-admin").querySelector(".play")!;
    await moveTo(splay);
    if (!alive()) return;
    await click(splay);
    if (!alive()) return;
    stop("prod-admin");
    say("<b>Stopped</b> prod-admin. Its credentials are gone.");
    await sleep(1000);
    if (!alive()) return;
    await moveTo(byId("contoso").querySelector(".play")!);
    if (!alive()) return;
    await click(byId("contoso").querySelector(".play")!);
    if (!alive()) return;
    stop("contoso");
    await sleep(1800);
    if (alive()) void demo();
  };

  // The window's shadow falls away from the pointer.
  let mx = -9999;
  let my = -9999;
  const resetShadow = () => {
    mx = -9999;
    my = -9999;
    app.style.setProperty("--shx", "0px");
    app.style.setProperty("--shy", "40px");
  };
  if (fine) {
    let frame = 0;
    const apply = () => {
      frame = 0;
      if (reduced) return;
      const rect = app.getBoundingClientRect();
      const x = (mx - (rect.left + rect.width / 2)) / innerWidth;
      const y = (my - (rect.top + rect.height / 2)) / innerHeight;
      app.style.setProperty("--shx", `${-x * 70}px`);
      app.style.setProperty("--shy", `${30 - y * 70}px`);
    };
    document.addEventListener("pointermove", (event) => {
      if (reduced) return;
      mx = event.clientX;
      my = event.clientY;
      if (!frame) frame = requestAnimationFrame(apply);
    });
    document.addEventListener("pointerleave", resetShadow);
  }
  motionPreference.addEventListener("change", (event) => {
    reduced = event.matches;
    if (!reduced) return;
    takeOver();
    resetShadow();
    ++typing;
    if (terminalId && term.classList.contains("open")) {
      renderTerm(termFor(terminalId));
    }
  });

  if (!reduced) initialDemo = setTimeout(() => void demo(), 1100);
}
