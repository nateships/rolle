# Rolle brand kit

Rolle uses the Original Circuit palette, a three-card stack with an open lowercase r, and a custom MuseoModerno wordmark.

| Use | Artwork | Source |
| --- | --- | --- |
| Desktop launcher, Dock and taskbar | Three-color stack with r cutout | [icons/](icons/) |
| System tray | Monochrome template stack | [icons/trayicon.svg](icons/trayicon.svg) |
| GitHub README | Go gopher peeking over the stack | [readme-gopher.png](readme-gopher.png) |
| Logo lockup | Stack with ivory or charcoal wordmark | [wordmark/](wordmark/) |

## Palette

![Rolle identity](wordmark/wordmark-preview.png)

The stack uses flat cobalt blue **#244CFF** at the back, orange **#FF7900** in the middle, and green **#00CE78** at the front. These are Rolle colors inspired by Azure, AWS, and GCP.

The interface uses neutral charcoal #101114, lifted surfaces #1E2024, warm ivory text #F4F0E8, neutral primary controls #E8EAED, and light-blue focus rings #8BA9FF. Bright colors are concentrated in the logo. Use solid fills with clear surface, border and secondary-text contrast. Credential states require explicit labels in addition to color.

Exact values live in [tokens.json](tokens.json) and [tokens.css](tokens.css).

## Typography and usage

The wordmark combines the icon’s exact r with MuseoModerno Bold (700) outlines. Use the supplied SVGs for the logo. Body/interface sans and command monospace have separate tokens. The unmodified font and license are in [fonts/](fonts/).

Desktop and tray icons use the stack alone. The Go gopher is reserved for the README mascot. Preserve its creator attribution and adaptation notice.

- [ICONS.md](ICONS.md): icon files, platform formats and regeneration.
- [TYPOGRAPHY.md](TYPOGRAPHY.md): wordmark masters, font usage and regeneration.
- [IMPLEMENTATION.md](IMPLEMENTATION.md): application integration and source locations.
- [ATTRIBUTION.md](ATTRIBUTION.md): Go gopher and font credits.
