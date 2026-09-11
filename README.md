<p align="center">
  <img src="docs/brand/readme-gopher.png" alt="Go gopher with Rolle credential cards and an r cutout" width="280">
</p>

# Rolle

Assume any role, any cloud.

Rolle is a desktop app and CLI that hands short-lived cloud credentials to your
tools without writing a secret to disk. It covers AWS, Azure, and Google Cloud.

## What it does

- **AWS**: sign in to IAM Identity Center once and get every account and role you
  can reach. Chain `AssumeRole` from any session. Add IAM users with optional MFA.
  Each active session is an AWS profile backed by `credential_process`, so every
  SDK and the AWS CLI pick it up with `--profile`.
- **Azure**: sign in to an Entra ID tenant with your Microsoft account. Every
  subscription becomes a session that yields Azure Resource Manager tokens.
- **Google Cloud**: reuse the credentials `gcloud` already has. Every project
  becomes a session, and you can impersonate service accounts.
- **Desktop app**: dark or light theme, guided onboarding, a dashboard with live
  expiry countdowns, favorites, one-click console and terminal actions, and
  signed self-updates.
- **Secrets** live in the OS keychain. Short-lived credentials are cached with
  owner-only permissions and expire on their own.
- **No telemetry.** The app and the CLI send nothing about you or your usage
  anywhere. They talk only to the cloud providers you sign in to and to GitHub
  releases for the update check, which you can turn off.

Rolle imports what your machine already has: Identity Center portals from the
AWS CLI and Granted, Azure tenants from the az CLI, gcloud credentials, and
sessions from a [Leapp](https://github.com/Noovolari/leapp) workspace.

## Install

```sh
brew install --cask nateships/tap/rolle   # macOS desktop app
brew install --cask nateships/tap/rolle-cli  # CLI
```

Windows and Linux desktop builds, and CLI archives for every platform, are on
the [releases page](https://github.com/nateships/rolle/releases). Docs live at
[getrolle.com](https://getrolle.com).

## CLI

```sh
# AWS Identity Center
rolle integration add aws-sso --alias acme --start-url https://acme.awsapps.com/start --region us-east-1
rolle integration login acme          # opens the browser, discovers roles
rolle session list
rolle start "Acme Prod/AdministratorAccess"
aws sts get-caller-identity --profile Acme-Prod-AdministratorAccess

# Role chaining and IAM users
rolle session add assume-role --name prod-admin --role-arn arn:aws:iam::123456789012:role/Admin --source "Acme Prod/AdministratorAccess" --region us-east-1
rolle session add iam-user --name personal --region us-east-1 --access-key-id AKIA...

# Azure
rolle integration add azure --alias contoso
rolle integration login contoso       # Microsoft sign-in, discovers subscriptions

# Google Cloud
gcloud auth application-default login
rolle integration add gcp             # discovers projects
rolle session add gcp-impersonate --name deployer --project my-project --service-account deployer@my-project.iam.gserviceaccount.com

# Any cloud
rolle start <session>
eval "$(rolle env <session>)"         # exports credentials into the shell
rolle shell <session>                 # opens a terminal with them ready
rolle console <session>               # opens the cloud console
rolle stop <session>
```

## Layout

- `internal/core`: domain model (integrations, sessions, credentials, workspace)
- `internal/app`: application layer shared by the CLI and desktop app
- `internal/aws`, `internal/azure`, `internal/gcp`: cloud providers
- `internal/discover`: import sources (AWS CLI, Granted, az CLI, gcloud, Leapp)
- `internal/awsconfig`, `internal/credcache`, `internal/secrets`, `internal/workspace`: storage
- `internal/terminal`: opens a terminal with a session's environment
- `cmd/rolle`: CLI
- `apps/desktop`: Wails v3 desktop app (React, Tailwind v4, shadcn/ui)
- `docs`: docs site for getrolle.com (Blume, built by Vercel with aube); `docs/brand` holds the brand kit

## Brand kit

The selected visual identity and coding-agent handoff live in
[docs/brand/README.md](docs/brand/README.md). Start there before changing logos,
app icons, typography, or brand colors.

README Go gopher artwork by [Renee French](https://go.dev/blog/gopher),
adapted for Rolle under [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/).

## Develop

```sh
mise install
mise run setup          # frontend deps + git hooks
mise run check          # go vet, lint, tests
mise run check:frontend # typecheck, lint, format, tests
mise run hooks          # every pre-commit hook on the whole tree
mise run desktop
```

Tools come from mise. Frontend and docs packages use [aube](https://aube.sh).
`mise run docs` serves the docs site locally.

Set `ROLLE_DEBUG=1` (or pass `rolle --debug`) for verbose diagnostics from the
CLI and the desktop app. `mise run reset` wipes the local workspace, secrets,
cache, and AWS profiles.

See [getrolle.com/roadmap](https://getrolle.com/roadmap) for what works and what does not.

## License

[GPL-3.0-or-later](LICENSE). Forks and redistributions must stay open under the same terms. The Go gopher artwork in the README is CC BY 4.0 (see above).
