import { useEffect, useMemo, useRef, useState } from "react";
import { AnimatePresence, motion } from "motion/react";
import {
  ArrowRight,
  Check,
  Cloud,
  Copy,
  Download,
  ExternalLink,
  Import,
  KeyRound,
  Loader2,
  RefreshCw,
  ShieldCheck,
  Sparkles,
  Terminal,
} from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { CommandLineInstall } from "@/components/CommandLine";
import { StaticKeysCard } from "@/components/StaticKeys";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { CloudGlyph, GopherLockup, GopherMark } from "@/components/Brand";
import { RegionSelect } from "@/components/RegionSelect";
import { DevTools } from "@/components/DevTools";
import {
  FoundList,
  tenantAlias,
  useDiscovery,
  type FoundPortal,
  type FoundTenant,
} from "@/components/discovery/FoundList";
import { Events } from "@wailsio/runtime";
import { api, errorMessage, inWails, type Session, type Workspace, type DeviceLogin, type GCPStatus } from "@/lib/api";
import { celebrate } from "@/lib/celebrate";
import { copyText } from "@/lib/clipboard";
import { cloudOf } from "@/lib/format";
import { cn } from "@/lib/utils";

type Step = "welcome" | "cloud" | "connect" | "approve" | "roles" | "done";
type CloudChoice = "aws" | "azure" | "gcp";
const ORDER: Step[] = ["welcome", "cloud", "connect", "approve", "roles", "done"];

type Accent = "blue" | "orange" | "green";
/** Each step borrows one of the three stack colors. */
const ACCENT: Record<Step, Accent> = {
  welcome: "blue",
  cloud: "blue",
  connect: "orange",
  approve: "green",
  roles: "blue",
  done: "green",
};
/** The connect step takes the color of the chosen cloud. */
const CLOUD_ACCENT: Record<CloudChoice, Accent> = { aws: "orange", azure: "blue", gcp: "green" };
const accentFor = (step: Step, cloud: CloudChoice): Accent => (step === "connect" ? CLOUD_ACCENT[cloud] : ACCENT[step]);
const ACCENT_TEXT: Record<Accent, string> = {
  blue: "text-brand-blue",
  orange: "text-brand-orange",
  green: "text-brand-green-text",
};
const ACCENT_BG: Record<Accent, string> = {
  blue: "bg-brand-blue",
  orange: "bg-brand-orange",
  green: "bg-brand-green",
};

const slide = {
  initial: { opacity: 0, x: 40, filter: "blur(4px)" },
  animate: { opacity: 1, x: 0, filter: "blur(0px)" },
  exit: { opacity: 0, x: -40, filter: "blur(4px)" },
  transition: { duration: 0.35, ease: [0.22, 1, 0.36, 1] as const },
};

export function Onboarding({ workspace }: { workspace: Workspace }) {
  // ?step= and ?cloud= let the browser preview jump into a step.
  const params = new URLSearchParams(location.search);
  const [step, setStep] = useState<Step>(
    (ORDER as string[]).includes(params.get("step") ?? "") ? (params.get("step") as Step) : "welcome",
  );
  const [cloud, setCloud] = useState<CloudChoice>(
    (["aws", "azure", "gcp"].includes(params.get("cloud") ?? "") ? params.get("cloud") : "aws") as CloudChoice,
  );
  const [method, setMethod] = useState<"sso" | "key">("sso");
  const [alias, setAlias] = useState("");
  const [login, setLogin] = useState<DeviceLogin | null>(
    params.get("step") === "approve" && params.get("cloud") !== "azure"
      ? { verificationUri: "#", userCode: "MOCK-CODE" }
      : null,
  );
  const [discovered, setDiscovered] = useState<Session[]>(
    params.get("step") === "roles" || params.get("step") === "done"
      ? workspace.sessions.length
        ? workspace.sessions
        : []
      : [],
  );
  const [busy, setBusy] = useState(false);

  const index = ORDER.indexOf(step);
  const go = (s: Step) => setStep(s);
  const connected = (c: CloudChoice) =>
    workspace.integrations.some((i) => i.cloud === c) || discovered.some((s) => cloudOf(s.kind) === c);

  // Identities other tools already set up on this machine, offered for one-click import.
  const found = useDiscovery(workspace, step === "cloud");
  // A cloud with an identity in the found list, before anything is imported.
  const foundFor = (c: CloudChoice) =>
    c === "aws" ? found.portals.length > 0 : c === "azure" ? found.tenants.length > 0 : !!found.gcp;
  const [importing, setImporting] = useState<string | null>(null);

  async function importPortal(p: FoundPortal) {
    setImporting(p.startUrl);
    setBusy(true);
    try {
      const res = await api.ImportAWSSSO(p.alias, p.startUrl, p.region);
      setAlias(p.alias);
      if (res.loggedIn) {
        setDiscovered((prev) => [...prev, ...(res.sessions ?? [])]);
        toast.success(`Reused your AWS CLI sign-in for ${p.alias}`);
        go("roles");
      } else {
        setCloud("aws");
        loginRef.current = res.integration.id;
        cancelledRef.current = false;
        const dl = await api.StartSSOLogin(res.integration.id);
        setLogin(dl);
        go("approve");
        const added = (await api.WaitSSOLogin(res.integration.id)) ?? [];
        loginRef.current = null;
        setDiscovered((prev) => [...prev, ...added]);
        go("roles");
      }
    } catch (e) {
      if (cancelledRef.current) return;
      toast.error(errorMessage(e));
      go("cloud");
    } finally {
      setImporting(null);
      setBusy(false);
    }
  }

  async function importAzure(t: FoundTenant) {
    setImporting(t.tenantId);
    setBusy(true);
    try {
      const integ = await api.AddAzure(tenantAlias(t), t.tenantId);
      setAlias(integ.alias);
      setCloud("azure");
      setLogin(null);
      go("approve");
      const added = (await api.AzureLogin(integ.id)) ?? [];
      setDiscovered((prev) => [...prev, ...added]);
      go("roles");
    } catch (e) {
      toast.error(errorMessage(e));
      go("cloud");
    } finally {
      setImporting(null);
      setBusy(false);
    }
  }

  async function importLeapp() {
    setImporting("leapp");
    setBusy(true);
    try {
      const res = await api.ImportLeappSessions();
      const added = res.sessions ?? [];
      setDiscovered((prev) => [...prev, ...added]);
      if (res.skipped?.length)
        toast.info(`${res.skipped.length} Leapp session${res.skipped.length === 1 ? "" : "s"} skipped`, {
          description: res.skipped[0],
        });
      if (added.length) go("roles");
      else found.rescan();
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setImporting(null);
      setBusy(false);
    }
  }

  async function importGCP() {
    setImporting("gcp");
    setBusy(true);
    try {
      const added = (await api.AddGCP("gcp")) ?? [];
      setDiscovered((prev) => [...prev, ...added]);
      go("roles");
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setImporting(null);
      setBusy(false);
    }
  }

  // Back and forward. The webview turns mouse back/forward buttons and trackpad
  // swipes into history navigation, so each step is a history entry and
  // popstate drives the step. Keyboard: Cmd/Ctrl+[ ], Alt+Left/Right.
  // Steps that need a completed sign-in are never skipped into, and a pending
  // approval is never abandoned.
  const BACK: Partial<Record<Step, Step>> = {
    cloud: "welcome",
    connect: "cloud",
    approve: "connect",
    roles: "cloud",
    done: "roles",
  };
  const FORWARD: Partial<Record<Step, Step>> = { welcome: "cloud", cloud: "connect", roles: "done" };
  const stepRef = useRef(step);
  stepRef.current = step;
  const busyRef = useRef(busy);
  busyRef.current = busy;

  // Every history entry carries a sequence number. A popstate compares the
  // popped number with the current one, which gives the direction even when
  // the flow loops back to an earlier step.
  const seqRef = useRef(0);
  const curSeqRef = useRef(0);
  const push = (s: Step) => {
    seqRef.current += 1;
    curSeqRef.current = seqRef.current;
    history.pushState({ step: s, n: seqRef.current }, "");
  };
  useEffect(() => {
    const current = (history.state as { step?: Step } | null)?.step;
    if (current !== step) push(step);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [step]);
  // The integration whose sign-in the approve step is waiting on.
  const loginRef = useRef<string | null>(null);
  const cancelledRef = useRef(false);
  const cancelLogin = () => {
    if (!loginRef.current) return;
    cancelledRef.current = true;
    void api.CancelSSOLogin(loginRef.current);
    loginRef.current = null;
  };

  useEffect(() => {
    const move = (dir: "back" | "forward") => {
      const from = stepRef.current;
      const target = dir === "back" ? BACK[from] : FORWARD[from];
      // Leaving the approve step abandons the sign-in; the approve step itself is not busy.
      if (from === "approve" && dir === "back" && target) {
        cancelLogin();
        setStep(target);
        return;
      }
      if (target && !busyRef.current) setStep(target);
      else push(from); // Keep the step. Push one entry so the next gesture has a target.
    };
    const onPop = (e: PopStateEvent) => {
      const n = (e.state as { n?: number } | null)?.n ?? 0;
      const dir = n < curSeqRef.current ? "back" : "forward";
      curSeqRef.current = n;
      move(dir);
    };
    const onKey = (e: KeyboardEvent) => {
      const mod = e.metaKey || e.ctrlKey;
      if ((mod && e.key === "[") || (e.altKey && e.key === "ArrowLeft")) {
        e.preventDefault();
        move("back");
      }
      if ((mod && e.key === "]") || (e.altKey && e.key === "ArrowRight")) {
        e.preventDefault();
        move("forward");
      }
    };
    const onMouseUp = (e: MouseEvent) => {
      if (e.button === 3) {
        e.preventDefault();
        move("back");
      }
      if (e.button === 4) {
        e.preventDefault();
        move("forward");
      }
    };
    window.addEventListener("popstate", onPop);
    window.addEventListener("keydown", onKey);
    window.addEventListener("mouseup", onMouseUp);
    // Native side forwards mouse buttons the webview swallows.
    const offBack = inWails ? Events.On("nav:back", () => move("back")) : () => {};
    const offForward = inWails ? Events.On("nav:forward", () => move("forward")) : () => {};
    return () => {
      window.removeEventListener("popstate", onPop);
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("mouseup", onMouseUp);
      offBack();
      offForward();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function finish() {
    try {
      await api.CompleteOnboarding();
    } catch (e) {
      toast.error(errorMessage(e));
    }
  }

  return (
    <div className="relative flex h-full flex-col overflow-hidden">
      <header className="drag relative z-10 flex items-center justify-between px-6 pt-5">
        <div className="mac-inset">
          <GopherLockup className="h-9" />
        </div>
        <div className="no-drag flex items-center gap-1.5">
          {ORDER.map((s, i) => (
            <motion.span
              key={s}
              layout
              className={cn(
                "h-1.5 rounded-full bg-muted-foreground/30",
                i === index && ACCENT_BG[accentFor(step, cloud)],
                i < index && "bg-foreground/50",
              )}
              animate={{ width: i === index ? 24 : 8 }}
              transition={{ type: "spring", stiffness: 300, damping: 30 }}
            />
          ))}
        </div>
        {/* Skip lands on the final step, so the command install is still offered.
            It also abandons a sign-in in progress, so a later approval cannot
            pull the user back. */}
        <Button
          variant="ghost"
          size="sm"
          className="no-drag text-muted-foreground"
          onClick={() => {
            cancelLogin();
            go("done");
          }}
          disabled={step === "done"}
        >
          Skip
        </Button>
      </header>

      <main className="relative z-10 flex flex-1 items-center justify-center overflow-y-auto px-8 py-6">
        <AnimatePresence mode="wait">
          {step === "welcome" && (
            <motion.section key="welcome" {...slide} className="flex max-w-xl flex-col items-center text-center">
              <motion.div
                initial={{ scale: 0.6, opacity: 0 }}
                animate={{ scale: 1, opacity: 1 }}
                transition={{ type: "spring", stiffness: 180, damping: 16 }}
                className="mb-8"
              >
                {/* The gopher greets with a dance once it has landed. */}
                <GopherMark className="size-28" autoplay="dance" />
              </motion.div>
              <h1 className="text-4xl font-semibold tracking-tight">
                Assume any <span className="text-brand-orange">role</span>, any{" "}
                <span className="text-brand-green">cloud</span>.
              </h1>
              <p className="mt-4 max-w-md text-balance text-muted-foreground">
                rolle keeps short-lived credentials flowing to your tools without writing a single secret to disk. Two
                minutes to set up.
              </p>
              <Button size="lg" className="mt-8 gap-2" onClick={() => go("cloud")}>
                Get started <ArrowRight className="size-4" />
              </Button>
            </motion.section>
          )}

          {step === "cloud" && (
            <motion.section key="cloud" {...slide} className="w-full max-w-2xl">
              <StepTitle
                eyebrow="Step 1"
                title={discovered.length ? "Add another cloud?" : "Where do your roles live?"}
                hint={
                  discovered.length
                    ? "Pick another provider, or connect AWS again with an access key."
                    : "You can add more clouds later."
                }
                accent="blue"
                highlight={discovered.length ? "another" : "roles"}
              />
              {!found.loading && found.count > 0 && (
                <div className="mt-6 rounded-2xl border bg-card p-4">
                  <div className="mb-3 flex items-center gap-2">
                    <Import className="size-4 text-brand-green-text" />
                    <p className="text-sm font-medium">Found on this machine</p>
                    <span className="text-xs text-muted-foreground">
                      From the AWS, Azure, and Google CLIs, Granted, and Leapp.
                    </span>
                  </div>
                  <FoundList
                    portals={found.portals}
                    tenants={found.tenants}
                    gcp={found.gcp}
                    leapp={found.leapp}
                    importing={importing}
                    disabled={busy}
                    onAWS={importPortal}
                    onAzure={importAzure}
                    onGCP={importGCP}
                    onLeapp={importLeapp}
                  />
                </div>
              )}
              <p className="mt-6 text-xs font-medium uppercase tracking-widest text-muted-foreground">
                {!found.loading && found.count > 0 ? "Or connect something new" : ""}
              </p>
              <div className="mt-3 grid grid-cols-3 gap-4">
                <CloudCard
                  cloud="aws"
                  title="Amazon Web Services"
                  desc="IAM Identity Center, assume role, IAM users"
                  connected={connected("aws")}
                  found={foundFor("aws")}
                  onClick={() => {
                    setCloud("aws");
                    go("connect");
                  }}
                />
                <CloudCard
                  cloud="azure"
                  title="Microsoft Azure"
                  desc="Entra ID tenants and subscriptions"
                  connected={connected("azure")}
                  found={foundFor("azure")}
                  onClick={() => {
                    setCloud("azure");
                    go("connect");
                  }}
                />
                <CloudCard
                  cloud="gcp"
                  title="Google Cloud"
                  desc="Projects and service account impersonation"
                  connected={connected("gcp")}
                  found={foundFor("gcp")}
                  onClick={() => {
                    setCloud("gcp");
                    go("connect");
                  }}
                />
              </div>
              {discovered.length > 0 && (
                <div className="mt-6 flex justify-center">
                  <Button
                    variant="secondary"
                    className="gap-2"
                    onClick={() => {
                      celebrate();
                      go("done");
                    }}
                  >
                    I'm done, finish setup <ArrowRight className="size-4" />
                  </Button>
                </div>
              )}
            </motion.section>
          )}

          {step === "connect" && cloud === "aws" && (
            <motion.section key="connect-aws" {...slide} className="w-full max-w-lg">
              <StepTitle
                eyebrow="Step 2"
                title="Connect to AWS"
                hint="Identity Center is the recommended path. It discovers every role you can reach."
                accent="orange"
                highlight="AWS"
              />
              <div className="mt-6 grid grid-cols-2 gap-3">
                <MethodButton
                  active={method === "sso"}
                  onClick={() => setMethod("sso")}
                  icon={<ShieldCheck className="size-4" />}
                  title="IAM Identity Center"
                  desc="Sign in once, get every role"
                />
                <MethodButton
                  active={method === "key"}
                  onClick={() => setMethod("key")}
                  icon={<KeyRound className="size-4" />}
                  title="Access key"
                  desc="A single IAM user"
                />
              </div>
              <div className="mt-6">
                {method === "sso" ? (
                  <SSOForm
                    busy={busy}
                    defaultRegion={workspace.settings?.defaultRegion ?? "us-east-1"}
                    onSubmit={async (v) => {
                      setBusy(true);
                      try {
                        const integ = await api.AddAWSSSO(v.alias, v.startUrl, v.region);
                        setAlias(integ.alias);
                        loginRef.current = integ.id;
                        cancelledRef.current = false;
                        const dl = await api.StartSSOLogin(integ.id);
                        setLogin(dl);
                        go("approve");
                        const added = (await api.WaitSSOLogin(integ.id)) ?? [];
                        if (cancelledRef.current) return;
                        loginRef.current = null;
                        setDiscovered((prev) => [...prev, ...added]);
                        go("roles");
                      } catch (e) {
                        if (cancelledRef.current) return;
                        toast.error(errorMessage(e));
                        go("connect");
                      } finally {
                        setBusy(false);
                      }
                    }}
                  />
                ) : (
                  <KeyForm
                    busy={busy}
                    defaultRegion={workspace.settings?.defaultRegion ?? "us-east-1"}
                    onSubmit={async (v) => {
                      setBusy(true);
                      try {
                        const s = await api.AddIAMUser(v);
                        setDiscovered((prev) => [...prev, s]);
                        go("roles");
                      } catch (e) {
                        toast.error(errorMessage(e));
                      } finally {
                        setBusy(false);
                      }
                    }}
                  />
                )}
              </div>
              <BackLink onClick={() => go("cloud")} />
            </motion.section>
          )}

          {step === "connect" && cloud === "azure" && (
            <motion.section key="connect-azure" {...slide} className="w-full max-w-lg">
              <StepTitle
                eyebrow="Step 2"
                title="Connect to Azure"
                hint="Sign in with your Microsoft account. rolle discovers every subscription you can see."
                accent="blue"
                highlight="Azure"
              />
              <div className="mt-6">
                <AzureForm
                  busy={busy}
                  onSubmit={async (v) => {
                    setBusy(true);
                    try {
                      const integ = await api.AddAzure(v.alias, v.tenant);
                      setAlias(integ.alias);
                      setLogin(null);
                      go("approve");
                      const added = (await api.AzureLogin(integ.id)) ?? [];
                      setDiscovered((prev) => [...prev, ...added]);
                      go("roles");
                    } catch (e) {
                      toast.error(errorMessage(e));
                      go("connect");
                    } finally {
                      setBusy(false);
                    }
                  }}
                />
              </div>
              <BackLink onClick={() => go("cloud")} />
            </motion.section>
          )}

          {step === "connect" && cloud === "gcp" && (
            <motion.section key="connect-gcp" {...slide} className="w-full max-w-lg">
              <StepTitle
                eyebrow="Step 2"
                title="Connect to Google Cloud"
                hint="rolle uses the credentials gcloud already has on this machine."
                accent="green"
                highlight="Google Cloud"
              />
              <div className="mt-6">
                <GCPConnect
                  busy={busy}
                  onSubmit={async (gcpAlias) => {
                    setBusy(true);
                    try {
                      const added = (await api.AddGCP(gcpAlias)) ?? [];
                      setDiscovered((prev) => [...prev, ...added]);
                      go("roles");
                    } catch (e) {
                      toast.error(errorMessage(e));
                    } finally {
                      setBusy(false);
                    }
                  }}
                />
              </div>
              <BackLink onClick={() => go("cloud")} />
            </motion.section>
          )}

          {step === "approve" && (
            <motion.section key="approve" {...slide} className="flex w-full max-w-lg flex-col items-center text-center">
              <StepTitle
                eyebrow="Step 3"
                title={login ? "Approve in your browser" : "Sign in with Microsoft"}
                hint={
                  login
                    ? login?.userCode
                      ? `The ${alias} sign-in page is open in your browser. Enter this code when the page asks for it.`
                      : `The ${alias} sign-in page is open in your browser. Approve the request there and come back.`
                    : `A browser window is open for ${alias}. Finish signing in there.`
                }
                center
                accent="green"
                highlight={login ? "browser" : "Microsoft"}
              />
              {login?.userCode ? (
                <motion.div
                  initial={{ scale: 0.9 }}
                  animate={{ scale: 1 }}
                  className="mt-8 rounded-2xl border border-brand-green/50 bg-card px-8 py-5 font-mono text-4xl font-semibold tracking-[0.3em]"
                >
                  {login.userCode}
                </motion.div>
              ) : login ? (
                <motion.div initial={{ scale: 0.9 }} animate={{ scale: 1 }} className="mt-8 rounded-2xl bg-card p-6">
                  <CloudGlyph cloud="aws" className="size-16 p-3" />
                </motion.div>
              ) : (
                <motion.div initial={{ scale: 0.9 }} animate={{ scale: 1 }} className="mt-8 rounded-2xl bg-card p-6">
                  <CloudGlyph cloud="azure" className="size-16 p-3" />
                </motion.div>
              )}
              <div className="mt-6 flex items-center gap-2 text-sm text-muted-foreground">
                <Loader2 className="size-4 animate-spin" /> Waiting for approval…
              </div>
              {login && (
                <div className="mt-2 flex items-center gap-4">
                  <Button
                    variant="link"
                    className="gap-1 text-muted-foreground"
                    onClick={() => void api.OpenURL(login.verificationUri)}
                  >
                    Reopen the page <ExternalLink className="size-3" />
                  </Button>
                  <Button
                    variant="link"
                    className="text-muted-foreground"
                    onClick={() => {
                      cancelLogin();
                      go("connect");
                    }}
                  >
                    Cancel
                  </Button>
                </div>
              )}
            </motion.section>
          )}

          {step === "roles" && (
            <motion.section key="roles" {...slide} className="w-full max-w-lg">
              <StepTitle
                eyebrow="Step 4"
                title={discovered.length === 1 ? "One session ready" : `${discovered.length} sessions ready`}
                hint="Start any of them from the dashboard or the CLI. You can connect more clouds before finishing."
                accent="blue"
                highlight={discovered.length === 1 ? "One session" : `${discovered.length} sessions`}
              />
              <ul className="mt-6 max-h-64 space-y-2 overflow-y-auto pr-1">
                {discovered.map((s, i) => (
                  <motion.li
                    key={s.id}
                    initial={{ opacity: 0, y: 12 }}
                    animate={{ opacity: 1, y: 0 }}
                    transition={{ delay: Math.min(i * 0.05, 0.6), type: "spring", stiffness: 260, damping: 24 }}
                    className="flex items-center gap-3 rounded-lg border bg-card px-3 py-2"
                  >
                    <CloudGlyph cloud={cloudOf(s.kind)} />
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium">{s.name}</p>
                      <p className="truncate text-xs text-muted-foreground">
                        {s.aws?.accountId ?? s.azure?.subscriptionId ?? s.gcp?.projectId ?? s.region}
                      </p>
                    </div>
                    <Check className="size-4 text-emerald-600 dark:text-emerald-400" />
                  </motion.li>
                ))}
                {discovered.length === 0 && (
                  <li className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
                    No roles found yet. You can add sessions from the dashboard.
                  </li>
                )}
              </ul>
              <div className="mt-6 grid grid-cols-2 gap-3">
                <Button variant="secondary" className="gap-2" onClick={() => go("cloud")}>
                  <Cloud className="size-4" /> Add another cloud
                </Button>
                <Button
                  className="gap-2"
                  onClick={() => {
                    celebrate();
                    go("done");
                  }}
                >
                  Finish <Sparkles className="size-4" />
                </Button>
              </div>
            </motion.section>
          )}

          {step === "done" && <Done key="done" count={discovered.length} onFinish={finish} />}
        </AnimatePresence>
      </main>
      <footer className="relative z-10 grid grid-cols-[1fr_auto_1fr] items-center px-6 pb-4 text-xs text-muted-foreground">
        <span />
        <span>
          {workspace.sessions.length > 0 && step !== "done"
            ? `${workspace.sessions.length} session(s) in your workspace`
            : "Secrets stay in your OS keychain."}
        </span>
        <span className="flex justify-end">
          <DevTools />
        </span>
      </footer>
    </div>
  );
}

function Done({ count, onFinish }: { count: number; onFinish: () => void }) {
  useEffect(() => {
    const t = setTimeout(() => celebrate("small"), 900);
    return () => clearTimeout(t);
  }, []);
  return (
    <motion.section {...slide} className="flex max-w-lg flex-col items-center text-center">
      <motion.div
        initial={{ scale: 0, rotate: -30 }}
        animate={{ scale: 1, rotate: 0 }}
        transition={{ type: "spring", stiffness: 200, damping: 12, delay: 0.1 }}
        className="mb-6 rounded-full bg-emerald-500/15 p-5 text-emerald-600 dark:text-emerald-400"
      >
        <Check className="size-12" strokeWidth={3} />
      </motion.div>
      <h2 className="text-3xl font-semibold tracking-tight">
        You're <span className="text-brand-green">all set</span>.
      </h2>
      <p className="mt-3 text-muted-foreground">
        {count > 0
          ? `${count} session${count === 1 ? "" : "s"} ready to start from the dashboard.`
          : "Your workspace is ready. Add clouds and sessions from the dashboard."}
      </p>
      <CommandLineInstall className="mt-6" />
      <StaticKeysCard className="mt-6 w-full" />
      <Button size="lg" className="mt-8 gap-2" onClick={onFinish}>
        Open dashboard <ArrowRight className="size-4" />
      </Button>
    </motion.section>
  );
}

/** Heading block: colored eyebrow, ivory title with one accented word, muted hint. */
function StepTitle({
  eyebrow,
  title,
  hint,
  center,
  accent = "blue",
  highlight,
}: {
  eyebrow: string;
  title: string;
  hint?: string;
  center?: boolean;
  accent?: Accent;
  highlight?: string;
}) {
  const parts = highlight && title.includes(highlight) ? title.split(highlight) : null;
  return (
    <div className={cn(center && "text-center")}>
      <p className={cn("text-xs font-semibold uppercase tracking-widest", ACCENT_TEXT[accent])}>{eyebrow}</p>
      <h2 className="mt-2 text-3xl font-semibold tracking-tight">
        {parts ? (
          <>
            {parts[0]}
            <span className={ACCENT_TEXT[accent]}>{highlight}</span>
            {parts.slice(1).join(highlight)}
          </>
        ) : (
          title
        )}
      </h2>
      {hint && <p className="mt-2 text-sm text-muted-foreground">{hint}</p>}
    </div>
  );
}

function CloudCard({
  cloud,
  title,
  desc,
  soon,
  connected,
  found,
  onClick,
}: {
  cloud: "aws" | "azure" | "gcp";
  title: string;
  desc: string;
  soon?: boolean;
  connected?: boolean;
  found?: boolean;
  onClick?: () => void;
}) {
  return (
    <motion.button
      type="button"
      whileHover={soon ? undefined : { y: -4 }}
      whileTap={soon ? undefined : { scale: 0.98 }}
      disabled={soon}
      onClick={onClick}
      className={cn(
        "flex flex-col items-start gap-3 rounded-2xl border bg-card p-5 text-left transition-colors",
        soon ? "opacity-60" : "hover:border-primary/50 hover:bg-card",
      )}
    >
      <div className="flex w-full items-center justify-between">
        <CloudGlyph cloud={cloud} className="size-12 p-2" />
        {soon ? (
          <Badge variant="secondary">Soon</Badge>
        ) : connected ? (
          <Badge variant="outline" className="gap-1 border-brand-green/40 bg-brand-green/10 text-brand-green-text">
            <Check className="size-3" /> Connected
          </Badge>
        ) : found ? (
          <Badge variant="outline" className="gap-1 text-muted-foreground">
            <Import className="size-3" /> Found
          </Badge>
        ) : (
          <Cloud className="size-4 text-muted-foreground" />
        )}
      </div>
      <div>
        <p className="font-medium">{title}</p>
        <p className="mt-1 text-xs text-muted-foreground">{desc}</p>
      </div>
    </motion.button>
  );
}

function MethodButton({
  active,
  onClick,
  icon,
  title,
  desc,
}: {
  active: boolean;
  onClick: () => void;
  icon: React.ReactNode;
  title: string;
  desc: string;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "flex items-start gap-3 rounded-lg border p-3 text-left transition-colors",
        active ? "border-primary bg-primary/10" : "hover:bg-accent",
      )}
    >
      <span
        className={cn(
          "mt-0.5 rounded-md p-1.5",
          active ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground",
        )}
      >
        {icon}
      </span>
      <span>
        <span className="block text-sm font-medium">{title}</span>
        <span className="block text-xs text-muted-foreground">{desc}</span>
      </span>
    </button>
  );
}

export function SSOForm({
  busy,
  onSubmit,
  submitLabel = "Sign in",
  defaultRegion = "us-east-1",
}: {
  busy: boolean;
  onSubmit: (v: { alias: string; startUrl: string; region: string }) => void;
  submitLabel?: string;
  defaultRegion?: string;
}) {
  const [alias, setAlias] = useState("");
  const [startUrl, setStartUrl] = useState("");
  const [region, setRegion] = useState(defaultRegion);
  const valid = useMemo(
    () => alias.trim() && /^https:\/\/.+/.test(startUrl.trim()) && region.trim(),
    [alias, startUrl, region],
  );
  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        if (valid) onSubmit({ alias: alias.trim(), startUrl: startUrl.trim(), region: region.trim() });
      }}
    >
      <Field label="Portal name" hint="How it shows in the sidebar">
        <Input value={alias} onChange={(e) => setAlias(e.target.value)} placeholder="acme" autoFocus />
      </Field>
      <Field label="Start URL">
        <Input
          value={startUrl}
          onChange={(e) => setStartUrl(e.target.value)}
          placeholder="https://acme.awsapps.com/start"
        />
      </Field>
      <Field label="Portal region" hint="The region shown in your Identity Center settings">
        <RegionSelect value={region} onChange={setRegion} />
      </Field>
      <Button type="submit" className="w-full gap-2" disabled={!valid || busy}>
        {busy ? <Loader2 className="size-4 animate-spin" /> : <ExternalLink className="size-4" />} {submitLabel}
      </Button>
    </form>
  );
}

export function KeyForm({
  busy,
  onSubmit,
  defaultRegion = "us-east-1",
}: {
  busy: boolean;
  onSubmit: (v: {
    name: string;
    region: string;
    accessKeyId: string;
    secretAccessKey: string;
    mfaDevice: string;
  }) => void;
  defaultRegion?: string;
}) {
  const [name, setName] = useState("");
  const [region, setRegion] = useState(defaultRegion);
  const [accessKeyId, setAccessKeyId] = useState("");
  const [secretAccessKey, setSecret] = useState("");
  const [mfaDevice, setMfa] = useState("");
  const valid = name.trim() && region.trim() && accessKeyId.trim() && secretAccessKey.trim();
  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        if (valid)
          onSubmit({
            name: name.trim(),
            region: region.trim(),
            accessKeyId: accessKeyId.trim(),
            secretAccessKey,
            mfaDevice: mfaDevice.trim(),
          });
      }}
    >
      <div className="grid grid-cols-2 gap-3">
        <Field label="Session name">
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="personal" autoFocus />
        </Field>
        <Field label="Region">
          <RegionSelect value={region} onChange={setRegion} />
        </Field>
      </div>
      <Field label="Access key ID">
        <Input
          value={accessKeyId}
          onChange={(e) => setAccessKeyId(e.target.value)}
          placeholder="AKIA…"
          className="font-mono"
        />
      </Field>
      <Field label="Secret access key" hint="Stored in your OS keychain">
        <Input
          type="password"
          value={secretAccessKey}
          onChange={(e) => setSecret(e.target.value)}
          className="font-mono"
          aria-label="Secret access key"
        />
      </Field>
      <Field label="MFA device (optional)">
        <Input
          value={mfaDevice}
          onChange={(e) => setMfa(e.target.value)}
          placeholder="arn:aws:iam::123456789012:mfa/me"
          className="font-mono text-xs"
        />
      </Field>
      <Button type="submit" className="w-full gap-2" disabled={!valid || busy}>
        {busy ? <Loader2 className="size-4 animate-spin" /> : <KeyRound className="size-4" />} Add session
      </Button>
    </form>
  );
}

function BackLink({ onClick }: { onClick: () => void }) {
  return (
    <button type="button" onClick={onClick} className="mt-4 text-xs text-muted-foreground hover:text-foreground">
      ← Choose a different cloud
    </button>
  );
}

export function AzureForm({
  busy,
  onSubmit,
  submitLabel = "Sign in with Microsoft",
}: {
  busy: boolean;
  onSubmit: (v: { alias: string; tenant: string }) => void;
  submitLabel?: string;
}) {
  const [alias, setAlias] = useState("");
  const [tenant, setTenant] = useState("");
  const valid = alias.trim().length > 0;
  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        if (valid) onSubmit({ alias: alias.trim(), tenant: tenant.trim() });
      }}
    >
      <Field label="Tenant name" hint="How it shows in the sidebar">
        <Input value={alias} onChange={(e) => setAlias(e.target.value)} placeholder="contoso" autoFocus />
      </Field>
      <Field label="Tenant ID or domain (optional)" hint="Leave empty to use your home tenant">
        <Input
          value={tenant}
          onChange={(e) => setTenant(e.target.value)}
          placeholder="contoso.onmicrosoft.com"
          className="font-mono text-xs"
        />
      </Field>
      <Button type="submit" className="w-full gap-2" disabled={!valid || busy}>
        {busy ? <Loader2 className="size-4 animate-spin" /> : <ExternalLink className="size-4" />} {submitLabel}
      </Button>
    </form>
  );
}

export function GCPConnect({ busy, onSubmit }: { busy: boolean; onSubmit: (alias: string) => void }) {
  const [alias, setAlias] = useState("gcp");
  const [status, setStatus] = useState<GCPStatus | null>(null);
  const [checking, setChecking] = useState(false);
  const [loggingIn, setLoggingIn] = useState(false);
  const [checkedAt, setCheckedAt] = useState<number | null>(null);

  const check = async (announce = false) => {
    setChecking(true);
    try {
      const next = await api.GCPStatus();
      setStatus(next);
      setCheckedAt(Date.now());
      if (announce) {
        if (next.ready) toast.success(`Found credentials for ${next.account || "your Google account"}`);
        else
          toast.info("Still no credentials", {
            description: "Run the command, finish the browser sign-in, then check again.",
          });
      }
      return next;
    } catch (e) {
      toast.error(errorMessage(e));
      return null;
    } finally {
      setChecking(false);
    }
  };
  // Check once on mount; `check` is recreated every render and must not re-trigger the effect.
  /* oxlint-disable react/exhaustive-effect-dependencies */
  useEffect(() => {
    void check();
  }, []);
  /* oxlint-enable react/exhaustive-effect-dependencies */

  const login = async () => {
    setLoggingIn(true);
    try {
      await api.GCloudLogin();
      const next = await check();
      if (next?.ready) {
        toast.success(`Signed in as ${next.account || "your Google account"}`);
        celebrate("small");
      }
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setLoggingIn(false);
    }
  };

  const copy = async (cmd: string) => {
    try {
      await copyText(cmd);
      toast.success("Command copied", { description: "Paste it into a terminal." });
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };

  return (
    <div className="space-y-4">
      {status === null ? (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="size-4 animate-spin" /> Looking for gcloud credentials…
        </div>
      ) : status.ready ? (
        <div className="flex items-center gap-3 rounded-lg border border-brand-green/40 bg-brand-green/5 px-3 py-2 text-sm">
          <Check className="size-4 text-brand-green-text" />
          <span className="truncate">
            Signed in as <span className="font-medium">{status.account || "a Google account"}</span>
          </span>
        </div>
      ) : (
        <div className="space-y-3 rounded-lg border border-dashed p-3 text-sm">
          {status.gcloudFound ? (
            <>
              <p>No Application Default Credentials yet. Sign in with gcloud and rolle will pick them up.</p>
              <Button className="w-full gap-2" onClick={login} disabled={loggingIn || checking}>
                {loggingIn ? <Loader2 className="size-4 animate-spin" /> : <Terminal className="size-4" />}
                {loggingIn ? "Waiting for gcloud…" : "Sign in with gcloud"}
              </Button>
            </>
          ) : (
            <>
              <p>
                The gcloud CLI is not installed, so rolle cannot sign you in. Install it, then come back and check
                again.
              </p>
              <Button className="w-full gap-2" onClick={() => void api.OpenURL(status.installUrl)}>
                <Download className="size-4" /> Install the gcloud CLI
              </Button>
            </>
          )}
          <div className="flex items-center gap-3 text-[11px] uppercase tracking-widest text-muted-foreground/70">
            <span className="h-px flex-1 bg-border" />
            or run it yourself
            <span className="h-px flex-1 bg-border" />
          </div>
          <div className="flex items-center gap-2">
            <code className="min-w-0 flex-1 truncate rounded bg-muted px-2 py-1.5 font-mono text-xs">
              {status.loginCommand}
            </code>
            <Button
              variant="outline"
              size="icon-sm"
              onClick={() => copy(status.loginCommand)}
              aria-label="Copy command"
            >
              <Copy />
            </Button>
          </div>
          <div className="flex items-center justify-between">
            <Button
              variant="secondary"
              size="sm"
              className="gap-1.5"
              onClick={() => void check(true)}
              disabled={checking || loggingIn}
            >
              {checking ? <Loader2 className="size-3.5 animate-spin" /> : <RefreshCw className="size-3.5" />}{" "}
              {checking ? "Checking…" : "Check again"}
            </Button>
            {checkedAt && (
              <span className="text-[11px] text-muted-foreground/70">
                Checked{" "}
                {new Date(checkedAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" })}
              </span>
            )}
          </div>
        </div>
      )}
      <Field label="Account name" hint="How it shows in the sidebar">
        <Input value={alias} onChange={(e) => setAlias(e.target.value)} />
      </Field>
      <Button
        className="w-full gap-2"
        disabled={!status?.ready || !alias.trim() || busy}
        onClick={() => onSubmit(alias.trim())}
      >
        {busy ? <Loader2 className="size-4 animate-spin" /> : <Cloud className="size-4" />} Discover projects
      </Button>
    </div>
  );
}

export function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div className="space-y-1.5">
      <Label className="text-xs text-muted-foreground">{label}</Label>
      {children}
      {hint && <p className="text-[11px] text-muted-foreground/70">{hint}</p>}
    </div>
  );
}
