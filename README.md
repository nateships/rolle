<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/public/logo-dark.svg">
    <img src="docs/public/logo-light.svg" alt="rolle: the Go gopher behind three credential cards, next to the rolle wordmark" width="420">
  </picture>
</p>

<p align="center">
  <a href="https://github.com/nateships/rolle/actions/workflows/ci.yml"><img src="https://github.com/nateships/rolle/actions/workflows/ci.yml/badge.svg?branch=main" alt="CI"></a>
  <a href="https://github.com/nateships/rolle/releases/latest"><img src="https://img.shields.io/github/v/release/nateships/rolle?label=release" alt="Latest release"></a>
  <a href="https://scorecard.dev/viewer/?uri=github.com/nateships/rolle"><img src="https://api.scorecard.dev/projects/github.com/nateships/rolle/badge" alt="OpenSSF Scorecard"></a>
  <a href="https://www.bestpractices.dev/projects/14603"><img src="https://www.bestpractices.dev/projects/14603/badge" alt="OpenSSF Best Practices"></a>
  <a href="https://docs.renovatebot.com/"><img src="https://img.shields.io/badge/renovate-enabled-brightgreen?logo=renovatebot" alt="Renovate enabled"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/nateships/rolle" alt="License"></a>
</p>

# rolle

Assume any role, any cloud.

<p align="center">
  <img src="docs/public/screenshots/dashboard-dark.png" alt="rolle dashboard: tags, AWS accounts with their roles, IAM users, an Azure subscription, and Google Cloud projects, two sessions active" width="900">
</p>

rolle is a desktop app and a CLI. It hands short-lived AWS, Azure, and Google Cloud credentials to your tools and writes no secret to disk.

- **AWS.** Sign in to IAM Identity Center once and get every account and role you can reach. Chain `AssumeRole` from any session. Add IAM users, with optional MFA. Each active session is a profile backed by `credential_process` for the AWS CLI and the SDKs.
- **Azure.** Sign in to an Entra ID tenant. Each subscription is a session that yields Resource Manager tokens.
- **Google Cloud.** Reuse the credentials `gcloud` has. Each project is a session. Impersonate service accounts.
- **Desktop app.** Expiry countdowns, favorites, tags, one-click console and terminal, a tray menu, and signed self-updates.
- **Secrets** live in the OS keychain. Short-lived credentials are owner-only files that expire.
- **No telemetry.** rolle connects only to the clouds you sign in to and to GitHub releases for the update check. The update check can be turned off.

rolle imports what your machine has: Identity Center portals from the AWS CLI and Granted, tenants from the az CLI, gcloud credentials, and a [Leapp](https://github.com/Noovolari/leapp) workspace.

## Install

```sh
brew install --cask nateships/tap/rolle
```

The cask installs the app and the `rolle` command. Windows and Linux builds are on the [releases page](https://github.com/nateships/rolle/releases). Docs: [getrolle.com](https://getrolle.com).

## CLI

```sh
rolle integration add aws-sso --alias acme --start-url https://acme.awsapps.com/start --region us-east-1
rolle integration login acme
rolle start "Acme Prod/AdministratorAccess"
aws sts get-caller-identity --profile default
```

`--json` and stable exit codes support scripts. See the [CLI reference](https://getrolle.com/docs/cli) and [Agents and scripts](https://getrolle.com/docs/agents).

## Support

Open an [issue](https://github.com/nateships/rolle/issues/new/choose). In the app, **Help → Report a problem** fills in your version and platform, and **Support bundle** writes a redacted zip to attach. It redacts account ids, emails, hosts, and secrets.

For a security problem, do not open an issue. Use [private vulnerability reporting](https://github.com/nateships/rolle/security/advisories/new). The [security policy](https://github.com/nateships/.github/blob/main/SECURITY.md) says what to include and what to expect. [Security model](https://getrolle.com/docs/security) describes what the project does today.

## Contributing

The [contributing guide](https://github.com/nateships/.github/blob/main/CONTRIBUTING.md) covers the commit convention, pull requests, and releases. The rest is specific to this repository.

### Setup

```sh
mise install
mise run setup          # frontend deps and git hooks
```

Tools come from [mise](https://mise.jdx.dev). Frontend and docs packages use [aube](https://aube.sh).

### Tasks

```sh
mise run check              # go vet, lint, tests
mise run check:frontend     # typecheck, lint, format, tests
mise run hooks              # every pre-commit hook on the whole tree
mise run desktop            # the desktop app in dev mode
mise run desktop:demo       # the same, on fictional data
mise run cli -- <args>      # the CLI from source, for example: mise run cli -- integration list
mise run capabilities:check # compare the Go capability graph with capslock.json; mise run capabilities rewrites it
mise run docs               # serve the docs site
mise run reset              # wipe the local workspace, secrets, cache, and AWS profiles
```

`ROLLE_DEBUG=1`, or `rolle --debug`, prints verbose diagnostics.

### Layout

- `internal/core`: domain model (integrations, sessions, credentials, workspace)
- `internal/app`: application layer shared by the CLI and the desktop app
- `internal/aws`, `internal/azure`, `internal/gcp`: cloud providers
- `internal/discover`: import sources (AWS CLI, Granted, az CLI, gcloud, Leapp)
- `internal/awsconfig`, `internal/credcache`, `internal/secrets`, `internal/workspace`: storage
- `internal/terminal`: opens a terminal with a session's environment
- `cmd/rolle`: CLI
- `apps/desktop`: Wails v3 desktop app (React, Tailwind v4, shadcn/ui)
- `docs`: the docs site for getrolle.com (Blume); `docs/brand` holds the brand kit

Read [docs/brand/README.md](docs/brand/README.md) before you change logos, app icons, typography, or brand colors.

## License

[GPL-3.0-or-later](LICENSE). The Go gopher artwork is by [Renee French](https://go.dev/blog/gopher), adapted for rolle under [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/).
