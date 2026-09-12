# Rolle brand kit

Rolle uses the Original Circuit palette, a three-card stack with an open lowercase r, and a custom MuseoModerno wordmark.

| Use | Artwork | Source |
| --- | --- | --- |
| Desktop launcher, Dock and taskbar | Three-color stack with r cutout | [icons/](icons/) |
| System tray | Monochrome template stack | [icons/trayicon.svg](icons/trayicon.svg) |
| Mascot lockup: README, docs site, app screens, social card | Go gopher peeking over the stack | [readme-gopher.png](readme-gopher.png) |
| Logo lockup | Stack with ivory or charcoal wordmark | [wordmark/](wordmark/) |

## Palette

![Rolle identity](wordmark/wordmark-preview.png)

The stack uses flat cobalt blue **#244CFF** at the back, orange **#FF7900** in the middle, and green **#00CE78** at the front. These are Rolle colors inspired by Azure, AWS, and GCP.

The interface uses neutral charcoal #101114, lifted surfaces #1E2024, warm ivory text #F4F0E8, neutral primary controls #E8EAED, and light-blue focus rings #8BA9FF. Bright colors are concentrated in the logo. Use solid fills with clear surface, border and secondary-text contrast. Credential states require explicit labels in addition to color.

Exact values live in [tokens.json](tokens.json) and [tokens.css](tokens.css).

## Typography and usage

The wordmark combines the icon’s exact r with MuseoModerno Bold (700) outlines. Use the supplied SVGs for the logo. Body/interface sans and command monospace have separate tokens. The unmodified font and license are in [fonts/](fonts/).

The app icon and the menu bar icon use the stack alone. Everywhere else the gopher lockup carries the brand. Preserve the creator attribution and adaptation notice.

- [ICONS.md](ICONS.md): icon files, platform formats and regeneration.
- [TYPOGRAPHY.md](TYPOGRAPHY.md): wordmark masters, font usage and regeneration.
- [IMPLEMENTATION.md](IMPLEMENTATION.md): application integration and source locations.
- [ATTRIBUTION.md](ATTRIBUTION.md): Go gopher and font credits.

## Social card

[og-card.html](og-card.html) is the source of the Open Graph image served as `docs/public/og-brand.png`. Render it at 1200x630 with a headless browser after a brand change.

## Gopher lockup

`docs/public/logo-dark.svg` and `logo-light.svg` combine the gopher artwork (background keyed out) with the wordmark. The getrolle.com header uses them (`docs/theme.css` sets the height), and the desktop app carries copies in `apps/desktop/frontend/src/assets/brand/` for its sidebar and onboarding headers.

## Gopher layers

`gopher.png` is the mascot with its background keyed out; `docs/public/logo-*.svg` embed it. The desktop app stacks two layers cut from it so the gopher can move on its own: `apps/desktop/frontend/src/assets/brand/gopher-cards.png` and `gopher-peek.png`. Regenerate them after a change to the artwork:

```sh
go run ./tools/gopherlayers docs/brand/gopher.png apps/desktop/frontend/src/assets/brand
```
