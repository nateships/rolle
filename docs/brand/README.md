# rolle brand kit

rolle uses the Original Circuit palette, a three-card stack with an open lowercase r, and a custom MuseoModerno wordmark.

| Use | Artwork | Source |
| --- | --- | --- |
| Desktop launcher, Dock and taskbar | Three-color stack with r cutout | [icons/](icons/) |
| System tray | Monochrome template stack | [icons/trayicon.svg](icons/trayicon.svg) |
| Gopher lockup: README, docs site, app | Go gopher peeking over the stack, next to the wordmark | [docs/public/logo-dark.svg](../public/logo-dark.svg), [logo-light.svg](../public/logo-light.svg) |
| Social card | Flattened gopher and stack on charcoal | [readme-gopher.png](readme-gopher.png) via [og-card.html](og-card.html) |
| Logo lockup | Stack with ivory or charcoal wordmark | [wordmark/](wordmark/) |

## Palette

![rolle identity](wordmark/wordmark-preview.png)

The stack uses flat cobalt blue **#244CFF** at the back, orange **#FF7900** in the middle, and green **#00CE78** at the front. These are rolle colors inspired by Azure, AWS, and GCP.

The interface uses neutral charcoal #101114, lifted surfaces #1E2024, warm ivory text #F4F0E8, neutral primary controls #E8EAED, and light-blue focus rings #8BA9FF. Bright colors are concentrated in the logo. Use solid fills with clear surface, border and secondary-text contrast. Credential states require explicit labels in addition to color.

Exact values live in [tokens.json](tokens.json) and [tokens.css](tokens.css).

## Typography and usage

The wordmark combines the icon’s exact r with MuseoModerno Bold (700) outlines. Use the supplied SVGs for the logo. Body/interface sans and command monospace have separate tokens. The unmodified font and license are in [fonts/](fonts/).

The app icon and the menu bar icon use the stack alone. Everywhere else the gopher lockup carries the brand. Preserve the creator attribution and adaptation notice.

- [ICONS.md](ICONS.md): icon files, platform formats and regeneration.
- [TYPOGRAPHY.md](TYPOGRAPHY.md): wordmark masters, font usage and regeneration.
- [ATTRIBUTION.md](ATTRIBUTION.md): Go gopher and font credits.

## Where the code uses it

- `tools/icons/main.go` is the source of the stack geometry and colors. It writes the SVGs and PNGs in `apps/desktop/build/`; see [ICONS.md](ICONS.md).
- `apps/desktop/frontend/src/index.css` maps [tokens.json](tokens.json) to the Tailwind theme. Change both together.
- `apps/desktop/frontend/src/components/Brand.tsx` draws the lockup: the vector rig from [animation-kit/](animation-kit/) as `GopherRig.tsx`, plus the wordmark SVGs copied from [wordmark/](wordmark/).
- The root README and the docs header use `docs/public/logo-dark.svg` and `logo-light.svg`. Keep the Renee French credit and the CC BY 4.0 notice in the README and in [ATTRIBUTION.md](ATTRIBUTION.md).

## Social card

[og-card.html](og-card.html) is the source of the Open Graph image served as `docs/public/og-brand.png`. Render it at 1200x630 with a headless browser after a brand change.

## Gopher lockup

`docs/public/logo-dark.svg` and `logo-light.svg` combine the gopher artwork (background keyed out) with the wordmark. The getrolle.com header uses them (`docs/theme.css` sets the height). The desktop app draws the mark from the vector rig instead; see Animation below.

## Animation

[animation-kit/](animation-kit/) holds the vector rig: the gopher and cards as named groups with pivots and holdout masks, plus an offline demo. The desktop app inlines the assembled master as `GopherRig.tsx`; a click on the mark plays a short dance or a hide-and-peek behind the green card.
