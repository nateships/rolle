# Desktop icon assets

Desktop icons use only the credential stack and open r. The gopher is reserved for the README mascot. No launcher or tray icon includes text or requires a font.

## Files

- icons/appicon.svg: primary three-color stack on a charcoal rounded tile, with transparent outer corners.
- icons/appicon-dark.svg: alias of the primary dark-tile icon.
- icons/appicon-light.svg: three-color stack on a warm ivory tile.
- icons/mark.svg: standalone three-color mark on transparency. Input for the wordmark builder.
- icons/trayicon.svg: black template mark on transparency.

The generated PNG, ICNS and ICO files live in apps/desktop/build/: appicon.png, trayicon.png, icons/ (launcher sizes and the 22px tray export), darwin/icons.icns, and windows/icon.ico. Linux packaging consumes build/appicon.png.

## Regenerate

From the repository root:

```sh
go run ./tools/icons
```

Then from apps/desktop:

```sh
wails3 task common:generate:icons
```

The Wails task also runs the Go generator itself, so the second command is sufficient for a full rebuild. The generator uses only the Go standard library and writes SVG and antialiased PNG from the same path definitions. Edit tools/icons/main.go as the source of truth, then refresh the SVGs in docs/brand/icons/ from the generated build files if the mark changes.

macOS uses the ICNS path in Info.plist. Bundle tasks clear stale Assets.car files in reused output bundles so the ICNS icon is used.
