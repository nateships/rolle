# Rolle brand kit

The selected identity has two uses. This is the current handoff for coding agents; ignore earlier brand assets and superseded generated wordmarks.

| Use | Artwork | Source |
| --- | --- | --- |
| Desktop launcher, Dock, taskbar and tray | Three stacked credential cards with an open lowercase r cutout; **no gopher and no wordmark** | [icons/](icons/) and ../../tools/icons/main.go |
| GitHub README and mascot artwork | Actual Go gopher peeking over the same stack and r | [readme-gopher.png](readme-gopher.png) |
| Wordmark when needed | Custom icon-matched r + MuseoModerno Bold outlines | [TYPOGRAPHY.md](TYPOGRAPHY.md) |

## Desktop icons

![Desktop icon](icons/appicon.png)

The primary icon is a dark stack on a lavender tile. The alternate dark icon uses a lavender stack on charcoal. The tray mark is black on true transparency for the existing Wails SetTemplateIcon call. All desktop variants omit the gopher. The r is a geometric cutout open to the bottom of the front card; it is not rendered from a font.

Production exports are in icons/ and wired into apps/desktop/build/. They include vector masters, 1024px PNGs, Windows ICO, macOS ICNS and 16–1024px raster sizes. See [ICONS.md](ICONS.md) for regeneration and platform details.

## README mascot

![README Go gopher](readme-gopher.png)

The Go gopher retains its large round eyes, tiny pupils and oval nose, round ears, two buck teeth and paws on the front card. It appears in the repository README, not the desktop launcher or tray. The README asset deliberately has no embedded wordmark so it cannot perpetuate the old generated lettering. See [ATTRIBUTION.md](ATTRIBUTION.md).

## Typography and color

**The wordmark now uses the icon’s exact r plus MuseoModerno Bold (700) outlines.** This implements the request to match the icon’s letterform and supersedes Space Grotesk. Use the production SVGs in [wordmark/](wordmark/), not typed text or old generated lettering. The unmodified font and license are in [fonts/](fonts/). See TYPOGRAPHY.md for the exact-logo versus stock-font distinction. The interface sans and command monospace remain separate tokens.

The palette is lavender #B8A1F2, charcoal #17151D, and warm ivory #F4F0E8. The exact color and typography values are in tokens.css and tokens.json. Use flat fills without glow, texture, or gradients in desktop artwork. Use explicit labels for credential states; the brand accent alone does not communicate status.

## Remaining references

- approved-direction.png records the historical gopher/stack composition; its gopher app-icon treatment and old font are superseded by the use rules above.
- references/03-stack-reference.png records the card and r concept.
- references/go-gopher-original.png is the original character reference.
- references/02-wordmark-reference.png is historical only; neither its armadillo nor its old lettering should be implemented.
- prompts/ records image-generation provenance, not current implementation instructions. This guide and TYPOGRAPHY.md take precedence over those historical prompts.
- IMPLEMENTATION.md describes integration boundaries and remaining work.
