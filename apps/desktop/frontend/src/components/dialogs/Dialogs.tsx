import { useEffect, useState } from "react";
import { ExternalLink, Loader2, LogIn } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { AzureForm, Field, GCPConnect, KeyForm, SSOForm } from "@/components/onboarding/Onboarding";
import { CloudGlyph } from "@/components/Brand";
import { RegionSelect } from "@/components/RegionSelect";
import { api, errorMessage, Cloud, type Integration, type Session, type Workspace, type DeviceLogin } from "@/lib/api";
import { celebrate } from "@/lib/celebrate";

export function AddSSODialog({
  open,
  onClose,
  onLogin,
}: {
  open: boolean;
  onClose: () => void;
  onLogin: (i: Integration) => void;
}) {
  const [busy, setBusy] = useState(false);
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add Identity Center portal</DialogTitle>
          <DialogDescription>Sign in once. rolle discovers every account and role you can reach.</DialogDescription>
        </DialogHeader>
        <SSOForm
          busy={busy}
          submitLabel="Add and sign in"
          onSubmit={async (v) => {
            setBusy(true);
            try {
              const integ = await api.AddAWSSSO(v.alias, v.startUrl, v.region);
              onLogin(integ);
            } catch (e) {
              toast.error(errorMessage(e));
            } finally {
              setBusy(false);
            }
          }}
        />
      </DialogContent>
    </Dialog>
  );
}

export function LoginDialog({
  integration,
  onClose,
  onDone,
}: {
  integration: Integration | null;
  onClose: () => void;
  onDone?: (integration: Integration, sessions: Session[]) => void;
}) {
  const [login, setLogin] = useState<DeviceLogin | null>(null);

  useEffect(() => {
    if (!integration) {
      setLogin(null);
      return;
    }
    let cancelled = false;
    (async () => {
      try {
        let sessions: Session[];
        if (integration.cloud === Cloud.CloudAzure) {
          sessions = (await api.AzureLogin(integration.id)) ?? [];
        } else {
          const dl = await api.StartSSOLogin(integration.id);
          if (cancelled) return;
          setLogin(dl);
          sessions = (await api.WaitSSOLogin(integration.id)) ?? [];
        }
        // An Azure sign-in has no cancel. It runs on after the dialog closes,
        // so its outcome still shows as a toast.
        const azure = integration.cloud === Cloud.CloudAzure;
        if (cancelled && !azure) return;
        if (!cancelled && sessions.length > 0) celebrate("small");
        toast.success(`Signed in to ${integration.alias}`, {
          description: sessions.length
            ? `${sessions.length} new session${sessions.length === 1 ? "" : "s"} discovered.`
            : "No new sessions.",
        });
        if (cancelled) return;
        onDone?.(integration, sessions);
        onClose();
      } catch (e) {
        if (cancelled && integration.cloud !== Cloud.CloudAzure) return;
        toast.error(errorMessage(e));
        if (!cancelled) onClose();
      }
    })();
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [integration?.id]);

  const cancel = () => {
    if (integration && integration.cloud !== Cloud.CloudAzure) void api.CancelSSOLogin(integration.id);
    onClose();
  };

  return (
    <Dialog open={!!integration} onOpenChange={(o) => !o && cancel()}>
      <DialogContent className="text-center">
        <DialogHeader className="items-center">
          <DialogTitle>Sign in to {integration?.alias ?? ""}</DialogTitle>
          <DialogDescription>
            {integration?.cloud === Cloud.CloudAzure
              ? "Finish signing in with Microsoft in your browser."
              : login?.userCode
                ? "Approve the request in your browser and confirm this code."
                : "Approve the request in your browser."}
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col items-center gap-4 py-2">
          {integration?.cloud === Cloud.CloudAzure || (login && !login.userCode) ? (
            <div className="rounded-2xl bg-card p-5">
              <CloudGlyph cloud={integration?.cloud === Cloud.CloudAzure ? "azure" : "aws"} className="size-14 p-2.5" />
            </div>
          ) : (
            <div className="rounded-2xl border border-brand-green/50 bg-card px-6 py-4 font-mono text-3xl font-semibold tracking-[0.3em]">
              {login?.userCode ?? "····-····"}
            </div>
          )}
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Loader2 className="size-4 animate-spin" /> Waiting for approval…
          </div>
          {login && (
            <div className="flex items-center gap-4">
              <Button
                variant="link"
                className="gap-1 text-muted-foreground"
                onClick={() => void api.OpenURL(login.verificationUri)}
              >
                Reopen the page <ExternalLink className="size-3" />
              </Button>
              <Button variant="link" className="text-muted-foreground" onClick={cancel}>
                Cancel
              </Button>
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}

export function AddAssumeRoleDialog({
  open,
  onClose,
  workspace,
}: {
  open: boolean;
  onClose: () => void;
  workspace: Workspace;
}) {
  const defaultRegion = workspace.settings?.defaultRegion ?? "us-east-1";
  const [name, setName] = useState("");
  const [roleArn, setRoleArn] = useState("");
  const [region, setRegion] = useState(defaultRegion);
  const [sourceRef, setSource] = useState("");
  const [externalId, setExternalId] = useState("");
  const [busy, setBusy] = useState(false);
  // The dialog stays mounted; clear the form when it closes.
  useEffect(() => {
    if (open) return;
    setName("");
    setRoleArn("");
    setRegion(defaultRegion);
    setSource("");
    setExternalId("");
  }, [open, defaultRegion]);
  // Only AWS sessions can provide the source credentials.
  const sources = workspace.sessions.filter((s) => s.kind.startsWith("aws"));
  const valid =
    name.trim() && /^arn:aws[a-z-]*:iam::\d{12}:role\/.+/.test(roleArn.trim()) && sourceRef && region.trim();
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Assume a role</DialogTitle>
          <DialogDescription>Chain from any AWS session you already have.</DialogDescription>
        </DialogHeader>
        <form
          className="space-y-4"
          onSubmit={async (e) => {
            e.preventDefault();
            if (!valid) return;
            setBusy(true);
            try {
              await api.AddAssumeRole({
                name: name.trim(),
                roleArn: roleArn.trim(),
                region: region.trim(),
                sourceRef,
                externalId: externalId.trim(),
                profile: "",
              });
              toast.success(`${name} added`);
              onClose();
            } catch (err) {
              toast.error(errorMessage(err));
            } finally {
              setBusy(false);
            }
          }}
        >
          <div className="grid grid-cols-2 gap-3">
            <Field label="Session name">
              <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="prod-admin" autoFocus />
            </Field>
            <Field label="Region">
              <RegionSelect value={region} onChange={setRegion} />
            </Field>
          </div>
          <Field label="Role ARN">
            <Input
              value={roleArn}
              onChange={(e) => setRoleArn(e.target.value)}
              placeholder="arn:aws:iam::123456789012:role/Admin"
              className="font-mono text-xs"
            />
          </Field>
          <Field label="Source session" hint="Provides the credentials for the AssumeRole call">
            <Select value={sourceRef} onValueChange={setSource}>
              <SelectTrigger className="w-full" aria-label="Source session">
                <SelectValue placeholder="Choose a session" />
              </SelectTrigger>
              <SelectContent>
                {sources.map((s) => (
                  <SelectItem key={s.id} value={s.id}>
                    {s.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          <Field label="External ID (optional)">
            <Input value={externalId} onChange={(e) => setExternalId(e.target.value)} className="font-mono text-xs" />
          </Field>
          <Button type="submit" className="w-full" disabled={!valid || busy}>
            {busy ? <Loader2 className="size-4 animate-spin" /> : "Add session"}
          </Button>
        </form>
      </DialogContent>
    </Dialog>
  );
}

export function AddIAMUserDialog({
  open,
  onClose,
  defaultRegion,
}: {
  open: boolean;
  onClose: () => void;
  defaultRegion?: string;
}) {
  const [busy, setBusy] = useState(false);
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add IAM user</DialogTitle>
          <DialogDescription>The secret key goes straight into your OS keychain.</DialogDescription>
        </DialogHeader>
        <KeyForm
          busy={busy}
          defaultRegion={defaultRegion}
          onSubmit={async (v) => {
            setBusy(true);
            try {
              await api.AddIAMUser(v);
              toast.success(`${v.name} added`);
              onClose();
            } catch (e) {
              toast.error(errorMessage(e));
            } finally {
              setBusy(false);
            }
          }}
        />
      </DialogContent>
    </Dialog>
  );
}

export function AddAWSLoginDialog({
  open,
  onClose,
  defaultRegion = "us-east-1",
}: {
  open: boolean;
  onClose: () => void;
  defaultRegion?: string;
}) {
  const [busy, setBusy] = useState(false);
  const [name, setName] = useState("");
  const [region, setRegion] = useState(defaultRegion);
  useEffect(() => {
    if (!open) {
      setName("");
      setRegion(defaultRegion);
    }
  }, [open, defaultRegion]);
  const valid = name.trim() && region.trim();
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add console login</DialogTitle>
          <DialogDescription>
            Sign in with your console credentials in the browser, the flow behind <code>aws login</code>. The first
            start opens the browser; no access key is stored.
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-4"
          onSubmit={async (e) => {
            e.preventDefault();
            if (!valid) return;
            setBusy(true);
            try {
              await api.AddAWSLogin({ name: name.trim(), region: region.trim() });
              toast.success(`${name.trim()} added`);
              onClose();
            } catch (err) {
              toast.error(errorMessage(err));
            } finally {
              setBusy(false);
            }
          }}
        >
          <div className="grid grid-cols-2 gap-3">
            <Field label="Session name">
              <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="console" autoFocus />
            </Field>
            <Field label="Region" hint="The sign-in region, and the default region for tools">
              <RegionSelect value={region} onChange={setRegion} />
            </Field>
          </div>
          <Button type="submit" className="w-full gap-2" disabled={!valid || busy}>
            {busy ? <Loader2 className="size-4 animate-spin" /> : <LogIn className="size-4" />} Add session
          </Button>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/** SessionLoginDialog runs the browser sign-in of a console login session. */
export function SessionLoginDialog({
  session,
  onClose,
  onDone,
}: {
  session: Session | null;
  onClose: () => void;
  onDone?: (session: Session) => void;
}) {
  const [login, setLogin] = useState<DeviceLogin | null>(null);

  useEffect(() => {
    if (!session) {
      setLogin(null);
      return;
    }
    let cancelled = false;
    (async () => {
      try {
        const dl = await api.StartSessionLogin(session.id);
        if (cancelled) return;
        setLogin(dl);
        const signed = await api.WaitSessionLogin(session.id);
        if (cancelled) return;
        toast.success(`Signed in to ${session.name}`, {
          description: signed.aws?.accountId ? `Account ${signed.aws.accountId}` : undefined,
        });
        onDone?.(signed);
        onClose();
      } catch (e) {
        if (cancelled) return;
        toast.error(errorMessage(e));
        onClose();
      }
    })();
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [session?.id]);

  const cancel = () => {
    if (session) void api.CancelSessionLogin(session.id);
    onClose();
  };

  return (
    <Dialog open={!!session} onOpenChange={(o) => !o && cancel()}>
      <DialogContent className="text-center">
        <DialogHeader className="items-center">
          <DialogTitle>Sign in to {session?.name ?? ""}</DialogTitle>
          <DialogDescription>Sign in with your console credentials in your browser.</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col items-center gap-4 py-2">
          <div className="rounded-2xl bg-card p-5">
            <CloudGlyph cloud="aws" className="size-14 p-2.5" />
          </div>
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Loader2 className="size-4 animate-spin" /> Waiting for the sign-in…
          </div>
          {login && (
            <div className="flex items-center gap-4">
              <Button
                variant="link"
                className="gap-1 text-muted-foreground"
                onClick={() => void api.OpenURL(login.verificationUri)}
              >
                Reopen the page <ExternalLink className="size-3" />
              </Button>
              <Button variant="link" className="text-muted-foreground" onClick={cancel}>
                Cancel
              </Button>
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}

export function MFADialog({
  open,
  onClose,
  onSubmit,
}: {
  open: boolean;
  onClose: () => void;
  onSubmit: (code: string) => void;
}) {
  const [code, setCode] = useState("");
  useEffect(() => {
    if (!open) setCode("");
  }, [open]);
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>MFA code</DialogTitle>
          <DialogDescription>Enter the one-time code from your device.</DialogDescription>
        </DialogHeader>
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (code.length >= 6) onSubmit(code.trim());
          }}
        >
          <Input
            value={code}
            onChange={(e) => setCode(e.target.value.replace(/\D/g, "").slice(0, 8))}
            inputMode="numeric"
            autoFocus
            className="text-center font-mono text-2xl tracking-[0.4em]"
            placeholder="000000"
          />
          <Button type="submit" className="w-full" disabled={code.length < 6}>
            Start session
          </Button>
        </form>
      </DialogContent>
    </Dialog>
  );
}

export function AddAzureDialog({
  open,
  onClose,
  onLogin,
}: {
  open: boolean;
  onClose: () => void;
  onLogin: (i: Integration) => void;
}) {
  const [busy, setBusy] = useState(false);
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add Azure tenant</DialogTitle>
          <DialogDescription>Sign in with Microsoft. rolle discovers every subscription you can see.</DialogDescription>
        </DialogHeader>
        <AzureForm
          busy={busy}
          submitLabel="Add and sign in"
          onSubmit={async (v) => {
            setBusy(true);
            try {
              const integ = await api.AddAzure(v.alias, v.tenant);
              onLogin(integ);
            } catch (e) {
              toast.error(errorMessage(e));
            } finally {
              setBusy(false);
            }
          }}
        />
      </DialogContent>
    </Dialog>
  );
}

export function AddGCPDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [busy, setBusy] = useState(false);
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add Google Cloud account</DialogTitle>
          <DialogDescription>
            Uses the Application Default Credentials gcloud already has on this machine.
          </DialogDescription>
        </DialogHeader>
        {open && (
          <GCPConnect
            busy={busy}
            onSubmit={async (alias) => {
              setBusy(true);
              try {
                const added = (await api.AddGCP(alias)) ?? [];
                toast.success(`${added.length} project${added.length === 1 ? "" : "s"} discovered`);
                if (added.length > 0) celebrate("small");
                onClose();
              } catch (e) {
                toast.error(errorMessage(e));
              } finally {
                setBusy(false);
              }
            }}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}

export function AddGCPImpersonationDialog({
  open,
  onClose,
  workspace,
}: {
  open: boolean;
  onClose: () => void;
  workspace: Workspace;
}) {
  const gcpIntegrations = workspace.integrations.filter((i) => i.gcp);
  const projects = Array.from(new Set(workspace.sessions.filter((s) => s.gcp).map((s) => s.gcp!.projectId))).sort();
  const [name, setName] = useState("");
  const [chosenIntegration, setIntegrationRef] = useState("");
  // The dialog mounts before any account exists, so derive the default on render.
  const integrationRef = chosenIntegration || (gcpIntegrations[0]?.id ?? "");
  const [projectId, setProjectId] = useState("");
  const [serviceAccount, setServiceAccount] = useState("");
  const [busy, setBusy] = useState(false);
  // The dialog stays mounted; clear the form when it closes.
  useEffect(() => {
    if (open) return;
    setName("");
    setIntegrationRef("");
    setProjectId("");
    setServiceAccount("");
  }, [open]);
  const valid =
    name.trim() &&
    integrationRef &&
    projectId.trim() &&
    /^[^@\s]+@[^@\s]+\.iam\.gserviceaccount\.com$/.test(serviceAccount.trim());
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Impersonate a service account</DialogTitle>
          <DialogDescription>Your Google account needs the Service Account Token Creator role on it.</DialogDescription>
        </DialogHeader>
        <form
          className="space-y-4"
          onSubmit={async (e) => {
            e.preventDefault();
            if (!valid) return;
            setBusy(true);
            try {
              await api.AddGCPImpersonation({
                name: name.trim(),
                integrationRef,
                projectId: projectId.trim(),
                serviceAccount: serviceAccount.trim(),
              });
              toast.success(`${name} added`);
              onClose();
            } catch (err) {
              toast.error(errorMessage(err));
            } finally {
              setBusy(false);
            }
          }}
        >
          <Field label="Session name">
            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="prod-deployer" autoFocus />
          </Field>
          {gcpIntegrations.length > 1 && (
            <Field label="Account">
              <Select value={integrationRef} onValueChange={setIntegrationRef}>
                <SelectTrigger className="w-full" aria-label="Account">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {gcpIntegrations.map((i) => (
                    <SelectItem key={i.id} value={i.id}>
                      {i.alias}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
          )}
          <Field label="Project ID">
            {projects.length > 0 ? (
              <Select value={projectId} onValueChange={setProjectId}>
                <SelectTrigger className="w-full" aria-label="Project ID">
                  <SelectValue placeholder="Choose a project" />
                </SelectTrigger>
                <SelectContent>
                  {projects.map((p) => (
                    <SelectItem key={p} value={p}>
                      {p}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : (
              <Input
                value={projectId}
                onChange={(e) => setProjectId(e.target.value)}
                placeholder="my-project"
                className="font-mono text-xs"
              />
            )}
          </Field>
          <Field label="Service account email">
            <Input
              value={serviceAccount}
              onChange={(e) => setServiceAccount(e.target.value)}
              placeholder="deployer@my-project.iam.gserviceaccount.com"
              className="font-mono text-xs"
            />
          </Field>
          <Button type="submit" className="w-full" disabled={!valid || busy}>
            {busy ? <Loader2 className="size-4 animate-spin" /> : "Add session"}
          </Button>
        </form>
      </DialogContent>
    </Dialog>
  );
}
