# Status

Rolle manages short-lived credentials for AWS, Azure, and Google Cloud from a
CLI and a desktop app. This page lists what works and what does not.

## Works

- **AWS**: IAM Identity Center sign-in with role discovery, `AssumeRole`
  chaining with optional external ID, IAM users with optional MFA, federated
  console links, `credential_process` profiles in `~/.aws/config`.
- **Azure**: Entra ID sign-in through the browser or a device code,
  subscription discovery, Azure Resource Manager tokens, portal links.
- **Google Cloud**: reuse of gcloud Application Default Credentials, project
  discovery, service account impersonation with an impersonated ADC file,
  console links.
- **Import**: Identity Center portals from the AWS CLI config and Granted,
  Azure tenants from the az CLI, gcloud credentials, and IAM users, chained
  roles, portals, and tenants from a Leapp workspace. A valid AWS CLI SSO token
  is reused so no second sign-in is needed.
- **Sessions**: start, stop, silent renewal, favorites, rename, copy
  credentials, open a terminal with the environment set, open the console.
- **Desktop app**: guided onboarding, dashboard, system tray with session
  toggles, settings (theme, default region, assume-role duration, terminal
  app, tray behaviour, logging, updates), signed self-updates from GitHub
  releases in release builds.
- **CLI**: `integration`, `session`, `start`, `stop`, `env`, `shell`,
  `console`, `status`, `reset`, and the hidden `creds` used by
  `credential_process`.
- **Release**: goreleaser for the CLI, per-platform desktop packages, and a
  signed update manifest, all from a tag push.

## Does not work yet

- Export of Azure tokens into the az CLI token cache. Use `rolle env` or the
  terminal action instead.
- Session expiry notifications. macOS requires a signed bundle for
  notifications.
- Code signing and notarization. Release builds are unsigned until the Apple
  and Windows signing secrets exist.
- Cascading removal of chained roles when their source session disappears.

## Out of scope

- Browser multi-session tooling. The AWS console provides it.
