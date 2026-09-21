# Desktop icon assets

Desktop icons use only the credential stack and open r. The gopher appears in the lockup (README, docs, app), never in a launcher or tray icon. No icon includes text or requires a font.

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
mise run icons
```

The task runs the Go generator, then packs the PNGs into the macOS ICNS and the Windows ICO. The outputs are committed and the desktop build uses them as they are, so run the task after a brand change and commit what it writes. The generator uses only the Go standard library and writes SVG and antialiased PNG from the same path definitions. Edit tools/icons/main.go as the source of truth, then refresh the SVGs in docs/brand/icons/ from the generated build files if the mark changes.

The ICNS and ICO bytes depend on the wails3 version that packs them. A wails3 bump can change them without a visible difference; rerun the task and commit the result when that happens.

macOS uses the ICNS path in Info.plist. Bundle tasks clear stale Assets.car files in reused output bundles so the ICNS icon is used.
