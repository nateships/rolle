# Coding-agent handoff

Read README.md, ICONS.md and TYPOGRAPHY.md in this folder first.

## Delivered

The desktop icon source is tools/icons/main.go. It generates the stack/r SVG and PNG exports without a gopher or lettering. The build's icon task generates Windows ICO and macOS ICNS, and Linux uses the PNG. The existing tray Go embed continues to consume the new 44px black template icon. Desktop build files and the brand kit contain matching exported copies.

The GitHub README uses readme-gopher.png. This generated raster artwork contains the actual Go gopher with the stack/r mark and no old wordmark. Keep its credit in the README and ATTRIBUTION.md. Do not substitute the gopher version into launcher or tray assets.

## Remaining work

Wordmark SVG masters and light/dark lockups are now in wordmark/. Use them directly: they combine the icon’s exact r with MuseoModerno Bold outlines. Space Grotesk is superseded. The unmodified MuseoModerno variable font, local font-face CSS and license are bundled in fonts/ for typeset brand text. See TYPOGRAPHY.md for regeneration and optical spacing. Do not trace lettering from the old generated board or retype the custom logo.

Map the supplied semantic color tokens into the existing React/Tailwind/shadcn theme when implementing the broader brand refresh. Review onboarding, tables, dialogs, credential countdowns, focus states and provider labels. Preserve functional success/warning/error semantics. Do not make unverified claims about how secrets or credential caches are stored.

For a future icon change, edit the Go generator and regenerate both build assets and the docs copies. Compare SVG/PNG rendering, transparent corners, the open r cutout and actual small sizes. Run relevant repository checks for any accompanying code change. Use ICONS.md rather than the superseded gopher app-icon mockup as the desktop source of truth.

No application theme rewrite, full app build, signing, git commit or push was performed for this asset handoff.
