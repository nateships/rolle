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

## 2. Desktop app
- [ ] Dark mode default, design tokens
- [ ] Walkthrough onboarding with animations and completion celebration
- [ ] Sessions dashboard: start, stop, copy credentials, open console
- [ ] Integration management

## 3. Azure
- [ ] Entra ID login (MSAL), subscription discovery, ARM tokens

## 4. GCP
- [ ] Google login, project discovery, service account impersonation, ADC file

## 5. Release
- [ ] goreleaser for CLI and daemon, desktop packaging, signing
