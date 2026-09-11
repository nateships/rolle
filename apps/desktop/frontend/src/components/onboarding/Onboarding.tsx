import { useEffect, useMemo, useState } from "react";
import { AnimatePresence, motion } from "motion/react";
import { ArrowRight, Check, Cloud, ExternalLink, KeyRound, Loader2, ShieldCheck, Sparkles } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Mark, CloudGlyph } from "@/components/Brand";
import { api, errorMessage, type Session, type Workspace } from "@/lib/api";
import { celebrate } from "@/lib/celebrate";
import { cloudOf } from "@/lib/format";
import { cn } from "@/lib/utils";

type Step = "welcome" | "cloud" | "connect" | "approve" | "roles" | "done";
type CloudChoice = "aws" | "azure" | "gcp";
const ORDER: Step[] = ["welcome", "cloud", "connect", "approve", "roles", "done"];

const slide = {
  initial: { opacity: 0, x: 40, filter: "blur(4px)" },
  animate: { opacity: 1, x: 0, filter: "blur(0px)" },
  exit: { opacity: 0, x: -40, filter: "blur(4px)" },
  transition: { duration: 0.35, ease: [0.22, 1, 0.36, 1] as const },
};

export function Onboarding({ workspace }: { workspace: Workspace }) {
  const [step, setStep] = useState<Step>("welcome");
  const [cloud, setCloud] = useState<CloudChoice>("aws");
  const [method, setMethod] = useState<"sso" | "key">("sso");
  const [alias, setAlias] = useState("");
  const [login, setLogin] = useState<{ verificationUri: string; userCode: string } | null>(null);
  const [discovered, setDiscovered] = useState<Session[]>([]);
  const [busy, setBusy] = useState(false);

  const index = ORDER.indexOf(step);
  const go = (s: Step) => setStep(s);

  async function finish() {
    try {
      await api.CompleteOnboarding();
    } catch (e) {
      toast.error(errorMessage(e));
    }
  }

  return (
    <div className="relative flex h-full flex-col overflow-hidden">
      <div className="grid-bg pointer-events-none absolute inset-0" />
      <header className="drag relative z-10 flex items-center justify-between px-6 pt-5">
        <div className="flex items-center gap-2 text-primary">
          <Mark className="size-6" />
          <span className="text-sm font-semibold tracking-tight text-foreground">Rolle</span>
        </div>
        <div className="no-drag flex items-center gap-1.5">
          {ORDER.map((s, i) => (
            <motion.span
              key={s}
              layout
              className={cn("h-1.5 rounded-full bg-muted-foreground/30", i === index && "bg-primary", i < index && "bg-primary/60")}
              animate={{ width: i === index ? 24 : 8 }}
              transition={{ type: "spring", stiffness: 300, damping: 30 }}
            />
          ))}
        </div>
        <Button variant="ghost" size="sm" className="no-drag text-muted-foreground" onClick={finish} disabled={step === "done"}>
          Skip
        </Button>
      </header>

      <main className="relative z-10 flex flex-1 items-center justify-center px-8 pb-10">
        <AnimatePresence mode="wait">
          {step === "welcome" && (
            <motion.section key="welcome" {...slide} className="flex max-w-xl flex-col items-center text-center">
              <motion.div initial={{ scale: 0.6, opacity: 0 }} animate={{ scale: 1, opacity: 1 }} transition={{ type: "spring", stiffness: 180, damping: 16 }} className="mb-8 rounded-full bg-primary/10 p-6 text-primary glow">
                <Mark className="size-20" animate />
              </motion.div>
              <h1 className="text-4xl font-semibold tracking-tight">Assume any role, any cloud.</h1>
              <p className="mt-4 max-w-md text-balance text-muted-foreground">
                Rolle keeps short-lived credentials flowing to your tools without writing a single secret to disk.
                Two minutes to set up.
              </p>
              <Button size="lg" className="mt-8 gap-2" onClick={() => go("cloud")}>
                Get started <ArrowRight className="size-4" />
              </Button>
            </motion.section>
          )}

          {step === "cloud" && (
            <motion.section key="cloud" {...slide} className="w-full max-w-2xl">
              <StepTitle eyebrow="Step 1" title="Where do your roles live?" hint="You can add more clouds later." />
              <div className="mt-8 grid grid-cols-3 gap-4">
                <CloudCard cloud="aws" title="Amazon Web Services" desc="IAM Identity Center, assume role, IAM users" onClick={() => { setCloud("aws"); go("connect"); }} />
                <CloudCard cloud="azure" title="Microsoft Azure" desc="Entra ID tenants and subscriptions" onClick={() => { setCloud("azure"); go("connect"); }} />
                <CloudCard cloud="gcp" title="Google Cloud" desc="Projects and service account impersonation" onClick={() => { setCloud("gcp"); go("connect"); }} />
              </div>
            </motion.section>
          )}

          {step === "connect" && cloud === "aws" && (
            <motion.section key="connect-aws" {...slide} className="w-full max-w-lg">
              <StepTitle eyebrow="Step 2" title="Connect to AWS" hint="Identity Center is the recommended path. It discovers every role you can reach." />
              <div className="mt-6 grid grid-cols-2 gap-3">
                <MethodButton active={method === "sso"} onClick={() => setMethod("sso")} icon={<ShieldCheck className="size-4" />} title="IAM Identity Center" desc="Sign in once, get every role" />
                <MethodButton active={method === "key"} onClick={() => setMethod("key")} icon={<KeyRound className="size-4" />} title="Access key" desc="A single IAM user" />
              </div>
              <div className="mt-6">
                {method === "sso" ? (
                  <SSOForm
                    busy={busy}
                    onSubmit={async (v) => {
                      setBusy(true);
                      try {
                        const integ = await api.AddAWSSSO(v.alias, v.startUrl, v.region);
                        setAlias(integ.alias);
                        const dl = await api.StartSSOLogin(integ.id);
                        setLogin(dl);
                        go("approve");
                        const added = await api.WaitSSOLogin(integ.id);
                        setDiscovered(added);
                        go("roles");
                      } catch (e) {
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
                    onSubmit={async (v) => {
                      setBusy(true);
                      try {
                        const s = await api.AddIAMUser(v);
                        setDiscovered([s]);
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
              <StepTitle eyebrow="Step 2" title="Connect to Azure" hint="Sign in with your Microsoft account. Rolle discovers every subscription you can see." />
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
                      const added = await api.AzureLogin(integ.id);
                      setDiscovered(added);
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
              <StepTitle eyebrow="Step 2" title="Connect to Google Cloud" hint="Rolle uses the credentials gcloud already has on this machine." />
              <div className="mt-6">
                <GCPConnect
                  busy={busy}
                  onSubmit={async (alias) => {
                    setBusy(true);
                    try {
                      const added = await api.AddGCP(alias);
                      setDiscovered(added);
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
              <StepTitle eyebrow="Step 3" title={login ? "Approve in your browser" : "Sign in with Microsoft"} hint={login ? `We opened ${alias}. Confirm this code when it asks.` : `A browser window is open for ${alias}. Finish signing in there.`} center />
              {login ? (
                <motion.div initial={{ scale: 0.9 }} animate={{ scale: 1 }} className="mt-8 rounded-xl border bg-card px-8 py-5 font-mono text-4xl font-semibold tracking-[0.3em] glow">
                  {login.userCode}
                </motion.div>
              ) : (
                <motion.div initial={{ scale: 0.9 }} animate={{ scale: 1 }} className="mt-8 rounded-full bg-sky-500/10 p-6 text-sky-300 glow">
                  <CloudGlyph cloud="azure" className="size-14 text-base" />
                </motion.div>
              )}
              <div className="mt-6 flex items-center gap-2 text-sm text-muted-foreground">
                <Loader2 className="size-4 animate-spin" /> Waiting for approval…
              </div>
              {login && (
                <Button variant="link" className="mt-2 gap-1 text-muted-foreground" onClick={() => void api.OpenURL(login.verificationUri)}>
                  Reopen the page <ExternalLink className="size-3" />
                </Button>
              )}
            </motion.section>
          )}

          {step === "roles" && (
            <motion.section key="roles" {...slide} className="w-full max-w-lg">
              <StepTitle eyebrow="Step 4" title={discovered.length === 1 ? "One session ready" : `${discovered.length} sessions discovered`} hint="Start any of them from the dashboard or the CLI." />
              <ul className="mt-6 max-h-64 space-y-2 overflow-y-auto pr-1">
                {discovered.map((s, i) => (
                  <motion.li
                    key={s.id}
                    initial={{ opacity: 0, y: 12 }}
                    animate={{ opacity: 1, y: 0 }}
                    transition={{ delay: Math.min(i * 0.05, 0.6), type: "spring", stiffness: 260, damping: 24 }}
                    className="flex items-center gap-3 rounded-lg border bg-card/60 px-3 py-2"
                  >
                    <CloudGlyph cloud={cloudOf(s.kind)} />
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium">{s.name}</p>
                      <p className="truncate text-xs text-muted-foreground">{s.aws?.accountId ?? s.azure?.subscriptionId ?? s.gcp?.projectId ?? s.region}</p>
                    </div>
                    <Check className="size-4 text-emerald-400" />
                  </motion.li>
                ))}
                {discovered.length === 0 && <li className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">No roles found yet. You can add sessions from the dashboard.</li>}
              </ul>
              <Button className="mt-6 w-full gap-2" onClick={() => { celebrate(); go("done"); }}>
                Finish <Sparkles className="size-4" />
              </Button>
            </motion.section>
          )}

          {step === "done" && <Done key="done" count={discovered.length} onFinish={finish} />}
        </AnimatePresence>
      </main>
      <footer className="relative z-10 px-6 pb-4 text-center text-xs text-muted-foreground">
        {workspace.sessions.length > 0 && step !== "done" ? `${workspace.sessions.length} session(s) in your workspace` : "Secrets stay in your OS keychain."}
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
      <motion.div initial={{ scale: 0, rotate: -30 }} animate={{ scale: 1, rotate: 0 }} transition={{ type: "spring", stiffness: 200, damping: 12, delay: 0.1 }} className="mb-6 rounded-full bg-emerald-500/15 p-5 text-emerald-400 ring-1 ring-emerald-500/30">
        <Check className="size-12" strokeWidth={3} />
      </motion.div>
      <h2 className="text-3xl font-semibold tracking-tight">You're all set.</h2>
      <p className="mt-3 text-muted-foreground">
        {count > 0 ? `${count} session${count === 1 ? "" : "s"} ready to start.` : "Your workspace is ready."} Start one, then use it from any terminal with <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs">rolle env &lt;name&gt;</code>.
      </p>
      <Button size="lg" className="mt-8 gap-2" onClick={onFinish}>
        Open dashboard <ArrowRight className="size-4" />
      </Button>
    </motion.section>
  );
}

function StepTitle({ eyebrow, title, hint, center }: { eyebrow: string; title: string; hint?: string; center?: boolean }) {
  return (
    <div className={cn(center && "text-center")}>
      <p className="text-xs font-medium uppercase tracking-widest text-primary">{eyebrow}</p>
      <h2 className="mt-2 text-3xl font-semibold tracking-tight">{title}</h2>
      {hint && <p className="mt-2 text-sm text-muted-foreground">{hint}</p>}
    </div>
  );
}

function CloudCard({ cloud, title, desc, soon, onClick }: { cloud: "aws" | "azure" | "gcp"; title: string; desc: string; soon?: boolean; onClick?: () => void }) {
  return (
    <motion.button
      type="button"
      whileHover={soon ? undefined : { y: -4 }}
      whileTap={soon ? undefined : { scale: 0.98 }}
      disabled={soon}
      onClick={onClick}
      className={cn("flex flex-col items-start gap-3 rounded-xl border bg-card/70 p-5 text-left transition-colors", soon ? "opacity-60" : "hover:border-primary/50 hover:bg-card")}
    >
      <div className="flex w-full items-center justify-between">
        <CloudGlyph cloud={cloud} />
        {soon ? <Badge variant="secondary">Soon</Badge> : <Cloud className="size-4 text-muted-foreground" />}
      </div>
      <div>
        <p className="font-medium">{title}</p>
        <p className="mt-1 text-xs text-muted-foreground">{desc}</p>
      </div>
    </motion.button>
  );
}

function MethodButton({ active, onClick, icon, title, desc }: { active: boolean; onClick: () => void; icon: React.ReactNode; title: string; desc: string }) {
  return (
    <button type="button" onClick={onClick} className={cn("flex items-start gap-3 rounded-lg border p-3 text-left transition-colors", active ? "border-primary bg-primary/10" : "hover:bg-accent")}>
      <span className={cn("mt-0.5 rounded-md p-1.5", active ? "bg-primary text-primary-foreground" : "bg-muted text-muted-foreground")}>{icon}</span>
      <span>
        <span className="block text-sm font-medium">{title}</span>
        <span className="block text-xs text-muted-foreground">{desc}</span>
      </span>
    </button>
  );
}

export function SSOForm({ busy, onSubmit, submitLabel = "Sign in" }: { busy: boolean; onSubmit: (v: { alias: string; startUrl: string; region: string }) => void; submitLabel?: string }) {
  const [alias, setAlias] = useState("");
  const [startUrl, setStartUrl] = useState("");
  const [region, setRegion] = useState("us-east-1");
  const valid = useMemo(() => alias.trim() && /^https:\/\/.+/.test(startUrl.trim()) && region.trim(), [alias, startUrl, region]);
  return (
    <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); if (valid) onSubmit({ alias: alias.trim(), startUrl: startUrl.trim(), region: region.trim() }); }}>
      <Field label="Portal name" hint="How it shows in the sidebar">
        <Input value={alias} onChange={(e) => setAlias(e.target.value)} placeholder="acme" autoFocus />
      </Field>
      <Field label="Start URL">
        <Input value={startUrl} onChange={(e) => setStartUrl(e.target.value)} placeholder="https://acme.awsapps.com/start" />
      </Field>
      <Field label="Portal region">
        <Input value={region} onChange={(e) => setRegion(e.target.value)} placeholder="us-east-1" />
      </Field>
      <Button type="submit" className="w-full gap-2" disabled={!valid || busy}>
        {busy ? <Loader2 className="size-4 animate-spin" /> : <ExternalLink className="size-4" />} {submitLabel}
      </Button>
    </form>
  );
}

export function KeyForm({ busy, onSubmit }: { busy: boolean; onSubmit: (v: { name: string; region: string; accessKeyId: string; secretAccessKey: string; mfaDevice: string }) => void }) {
  const [name, setName] = useState("");
  const [region, setRegion] = useState("us-east-1");
  const [accessKeyId, setAccessKeyId] = useState("");
  const [secretAccessKey, setSecret] = useState("");
  const [mfaDevice, setMfa] = useState("");
  const valid = name.trim() && region.trim() && accessKeyId.trim() && secretAccessKey.trim();
  return (
    <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); if (valid) onSubmit({ name: name.trim(), region: region.trim(), accessKeyId: accessKeyId.trim(), secretAccessKey, mfaDevice: mfaDevice.trim() }); }}>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Session name"><Input value={name} onChange={(e) => setName(e.target.value)} placeholder="personal" autoFocus /></Field>
        <Field label="Region"><Input value={region} onChange={(e) => setRegion(e.target.value)} /></Field>
      </div>
      <Field label="Access key ID"><Input value={accessKeyId} onChange={(e) => setAccessKeyId(e.target.value)} placeholder="AKIA…" className="font-mono" /></Field>
      <Field label="Secret access key" hint="Stored in your OS keychain"><Input type="password" value={secretAccessKey} onChange={(e) => setSecret(e.target.value)} className="font-mono" /></Field>
      <Field label="MFA device (optional)"><Input value={mfaDevice} onChange={(e) => setMfa(e.target.value)} placeholder="arn:aws:iam::123456789012:mfa/me" className="font-mono text-xs" /></Field>
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

export function AzureForm({ busy, onSubmit, submitLabel = "Sign in with Microsoft" }: { busy: boolean; onSubmit: (v: { alias: string; tenant: string }) => void; submitLabel?: string }) {
  const [alias, setAlias] = useState("");
  const [tenant, setTenant] = useState("");
  const valid = alias.trim().length > 0;
  return (
    <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); if (valid) onSubmit({ alias: alias.trim(), tenant: tenant.trim() }); }}>
      <Field label="Tenant name" hint="How it shows in the sidebar">
        <Input value={alias} onChange={(e) => setAlias(e.target.value)} placeholder="contoso" autoFocus />
      </Field>
      <Field label="Tenant ID or domain (optional)" hint="Leave empty to use your home tenant">
        <Input value={tenant} onChange={(e) => setTenant(e.target.value)} placeholder="contoso.onmicrosoft.com" className="font-mono text-xs" />
      </Field>
      <Button type="submit" className="w-full gap-2" disabled={!valid || busy}>
        {busy ? <Loader2 className="size-4 animate-spin" /> : <ExternalLink className="size-4" />} {submitLabel}
      </Button>
    </form>
  );
}

export function GCPConnect({ busy, onSubmit }: { busy: boolean; onSubmit: (alias: string) => void }) {
  const [alias, setAlias] = useState("gcp");
  const [status, setStatus] = useState<{ ready: boolean; account: string; loginCommand: string } | null>(null);
  const check = async () => setStatus(await api.GCPStatus());
  useEffect(() => { void check(); }, []);
  return (
    <div className="space-y-4">
      {status === null ? (
        <div className="flex items-center gap-2 text-sm text-muted-foreground"><Loader2 className="size-4 animate-spin" /> Looking for gcloud credentials…</div>
      ) : status.ready ? (
        <div className="flex items-center gap-3 rounded-lg border border-emerald-500/30 bg-emerald-500/5 px-3 py-2 text-sm">
          <Check className="size-4 text-emerald-400" />
          <span className="truncate">Signed in as <span className="font-medium">{status.account || "a Google account"}</span></span>
        </div>
      ) : (
        <div className="space-y-2 rounded-lg border border-dashed p-3 text-sm">
          <p>No Application Default Credentials found. Run this in a terminal, then check again:</p>
          <code className="block rounded bg-muted px-2 py-1.5 font-mono text-xs">{status.loginCommand}</code>
          <Button variant="secondary" size="sm" onClick={() => void check()}>Check again</Button>
        </div>
      )}
      <Field label="Account name" hint="How it shows in the sidebar">
        <Input value={alias} onChange={(e) => setAlias(e.target.value)} />
      </Field>
      <Button className="w-full gap-2" disabled={!status?.ready || !alias.trim() || busy} onClick={() => onSubmit(alias.trim())}>
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
