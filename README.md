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
- **Desktop app**: dark by default, guided onboarding, a dashboard with live
  expiry countdowns, one-click console links, and copy-to-clipboard credentials.
- **Secrets** live in the OS keychain. Short-lived credentials are cached with
  owner-only permissions and expire on their own.

Rolle is a successor to [Leapp](https://github.com/Noovolari/leapp), which is no
longer maintained. It does not replicate browser multi-session tooling; AWS now
ships that natively.

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
rolle console <session>               # opens the cloud console
rolle stop <session>
```

## Layout

- `internal/core`: domain model (integrations, sessions, credentials, workspace)
- `internal/app`: application layer shared by the CLI and desktop app
- `internal/aws`, `internal/azure`, `internal/gcp`: cloud providers
- `internal/awsconfig`, `internal/credcache`, `internal/secrets`, `internal/workspace`: storage
- `cmd/rolle`: CLI
- `apps/desktop`: Wails v3 desktop app (React, Tailwind v4, shadcn/ui)

## Develop

```sh
mise install
mise run setup
mise run check
mise run desktop
```

Tools come from mise. Frontend packages use [aube](https://aube.sh).
