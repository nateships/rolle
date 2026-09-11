# Rolle

Assume any role, any cloud.

Rolle is a desktop app and CLI that manages short-lived cloud credentials.
It is a Go successor to [Leapp](https://github.com/Noovolari/leapp), which is no longer maintained.

## Layout

- `internal/core`: sessions, provider interface, credential storage
- `cmd/rolled`: background daemon that owns sessions and refresh timers
- `cmd/rolle`: CLI, thin client for the daemon
- `apps/desktop`: Wails v3 desktop app (React, Tailwind v4, shadcn/ui)

## Develop

```sh
mise install
mise run setup
mise run check
mise run desktop
```

Tools come from mise. Frontend packages use [aube](https://aube.sh).
