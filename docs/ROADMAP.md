# Roadmap

Scope: role assumption across AWS, Azure, and GCP with a CLI and a desktop app.
Out of scope: browser multi-session tooling (AWS provides this natively).

## 1. Core and CLI
- [x] Domain model: integrations, sessions, credentials, workspace
- [x] Workspace file (versioned JSON, atomic writes)
- [x] Secret store (OS keychain, in-memory for tests)
- [x] AWS IAM Identity Center: device-flow login, list accounts and roles, role credentials
- [x] AWS AssumeRole with session chaining and optional external ID
- [x] AWS IAM user with optional MFA
- [x] `credential_process` entry point and `~/.aws/config` profile management
- [x] Credential cache with expiry
- [x] CLI: integration, session, start, stop, creds, env, console
- [x] Silent renewal of expired sessions on refresh

## 2. Desktop app
- [x] Dark mode default, design tokens
- [x] Walkthrough onboarding with animations and completion celebration
- [x] Sessions dashboard: start, stop, copy credentials, open console
- [x] Integration management

## 3. Azure
- [x] Entra ID login (MSAL), subscription discovery, ARM tokens
- [ ] Export tokens into the az CLI cache

## 4. GCP
- [x] gcloud ADC reuse, project discovery, service account impersonation
- [x] Write impersonated ADC file for SDKs without env support

## 5. Release
- [ ] goreleaser for the CLI, desktop packaging, signing
- [x] System tray with quick start and stop
- [ ] Expiry notifications (needs a signed bundle on macOS)
