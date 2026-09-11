# Desktop icon assets

Desktop icons use only the credential stack and open r. The gopher is reserved for the README mascot. No launcher or tray icon includes text or requires a font.

## Files

- icons/appicon.svg and appicon.png: primary three-color stack on a charcoal rounded tile, with transparent outer corners.
- icons/appicon-dark.svg and appicon-dark.png: alias of the primary dark-tile icon.
- icons/appicon-light.svg and appicon-light.png: three-color stack on a warm ivory tile.
- icons/mark.svg and mark.png: standalone three-color mark on transparency.
- icons/trayicon.svg and trayicon.png: black template mark on transparency; PNG is 44px for the existing tray integration.
- icons/rolle.icns: native macOS icon container.
- icons/rolle.ico: Windows sizes 16, 24, 32, 48, 64, 128 and 256px.
- icons/sizes/: launcher PNGs at 16, 24, 32, 48, 64, 128, 256, 512 and 1024px, plus a 22px tray export.

The build copies live at apps/desktop/build/appicon.png, trayicon.png, darwin/icons.icns, and windows/icon.ico. Linux packaging already consumes build/appicon.png.

## Regenerate

From the repository root:

```sh
go run ./tools/icons
```

Then from apps/desktop:

```sh
wails3 task common:generate:icons
```

The Wails task also runs the Go generator itself, so the second command is sufficient for a full rebuild. The generator uses only the Go standard library and writes SVG and antialiased PNG from the same path definitions. Edit tools/icons/main.go as the source of truth, then refresh docs/brand/icons/ from the generated build files if the mark changes.

macOS uses the ICNS path in Info.plist. Bundle tasks clear stale Assets.car files in reused output bundles so the ICNS icon is used.

## Verification

The generator was run, the native containers were produced with the installed Wails CLI, and PNG/SVG consistency, sizes, alpha, r-cutout pixels and small-size previews were checked. This asset update does not constitute a full application build or a signed package.
