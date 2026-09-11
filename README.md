# Rolle

Assume any role, any cloud.

Rolle is a desktop app and CLI that manages short-lived cloud credentials.
It is a Rust successor to [Leapp](https://github.com/Noovolari/leapp), which is no longer maintained.

## Layout

- `crates/rolle-core`: sessions, provider trait, credential storage
- `crates/rolled`: background daemon that owns sessions and refresh timers
- `crates/rolle`: CLI, thin client for the daemon
- `apps/desktop`: Tauri 2 desktop app (Svelte)

## Develop

```sh
mise install
mise run setup
mise run check
mise run desktop
```

Cargo builds run through [mr. boxington](https://mr-boxington.jdx.dev/). Frontend packages use [aube](https://aube.sh). Both come from mise.
