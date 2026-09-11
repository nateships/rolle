# Brand integration

Read [README.md](README.md), [ICONS.md](ICONS.md) and [TYPOGRAPHY.md](TYPOGRAPHY.md) for the identity and asset rules. Code paths below are relative to the repository root.

## Icons

`tools/icons/main.go` is the source for the stack geometry and three card colors. It generates SVGs and antialiased PNG files in `apps/desktop/build/`. The desktop build task also generates Windows ICO and macOS ICNS; Linux consumes the PNG. The tray uses a 44px black template mark.

After changing the generator, refresh the build assets and matching copies in `docs/brand/icons/`. Check transparent corners, the open r cutout and small sizes. Keep desktop and tray artwork free of the gopher and wordmark.

## Wordmark and mascot

Use the outlined masters in `docs/brand/wordmark/`. They combine the icon’s exact r with MuseoModerno Bold outlines. The font, local font-face CSS and license are bundled in `docs/brand/fonts/` for typeset brand text.

The root README uses `docs/brand/readme-gopher.png`. Preserve the Renee French credit, CC BY 4.0 link and adaptation notice in the README and `ATTRIBUTION.md`.

## Interface

`apps/desktop/frontend/src/index.css` maps the brand palette to the React/Tailwind/shadcn dark theme. Keep it synchronized with `docs/brand/tokens.json` and `tokens.css`.

`apps/desktop/frontend/src/components/Brand.tsx` contains the React mark and wordmark. Its card paths and colors must match the Go icon generator; its wordmark paths must match the outlined SVG master.

Use neutral surfaces and controls with strong text contrast. Preserve functional success, warning and error semantics, and identify credential states with explicit labels.
