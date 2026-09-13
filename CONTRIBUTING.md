# Contributing

## Setup

```sh
mise install
mise run setup          # frontend deps and git hooks
```

Tools come from [mise](https://mise.jdx.dev). Frontend and docs packages use [aube](https://aube.sh).

## Tasks

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

## Changes

Changes reach `main` through pull requests that pass CI. Commit subjects follow [Conventional Commits](https://www.conventionalcommits.org): `feat(cli):`, `fix(app):`, `docs:`, `chore:`. A commit-msg hook checks them.

[release-please](https://github.com/googleapis/release-please) keeps a release pull request open from those commits. Merging it tags the release and starts the build. Until 1.0, a feature bumps the patch version.

## Layout

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
