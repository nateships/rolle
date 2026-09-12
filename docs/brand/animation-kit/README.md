# Rolle gopher animation kit

This is a vector reconstruction for animation, following your existing ivory gopher, dark outlines and Original Circuit colors. It is a reviewable starting rig, not a lossless separation of the PNG. The hidden body, legs, arms and completed rear card surfaces are newly drawn. Your repository and production assets have not been modified.

## What the asset review found

- `docs/brand/icons/mark.svg` and `tools/icons/main.go` already contain three separate vector paths. The blue and orange shapes are visible strips, not full squares.
- `docs/brand/readme-gopher.png` is a flattened mascot and stack on a dark background. `apps/desktop/frontend/src/assets/brand/gopher.png` is a small transparent version of the entire lockup, not the gopher alone.
- `docs/public/logo-dark.svg` and `logo-light.svg` embed raster mascot artwork. An SVG filename alone does not make the gopher editable.
- The mascot stack and launcher stack have different proportions. This kit keeps the original launcher geometry in a separate file. Do not substitute its 512-unit paths directly into the 385-unit mascot canvas.
- The existing face is recognizable through its oversized eyes, leftward pupils, small nose, two teeth, ivory fill, and dark outlines. The reconstruction preserves those features, with flat fills replacing the source texture and slightly regularized curves.
- Brand documentation disagrees about mascot placement: `ICONS.md` says README-only, while the current brand README and React component use it on the website and app screens. This kit follows the existing artwork; that documentation could be reconciled separately.

## Files

| File | Use |
| --- | --- |
| `rolle-gopher-layered.svg` | Assembled, transparent mascot lockup with named editable groups. |
| `rig.json` | Layer names, coordinate systems, pivots and default stacking order. |
| `preview.html` | Offline interactive demo: dance, hide, peek behind each card, separate layers. |
| `ATTRIBUTION.md` | Original project artwork and font attribution. |

The assembled master uses named groups for ears, feet, arms, body, eye whites, pupils, teeth, muzzle, nose, paws and each card. Each part is editable vector geometry. The desktop app inlines this master as `apps/desktop/frontend/src/components/GopherRig.tsx`; regenerate that component from the master after a change.

## Layer order and occlusion

Default paint order, back to front:

1. Blue card.
2. Orange card.
3. `gopher-depth` wrapper containing the animated `gopher-root`.
4. Green card.
5. `hands-root`, so the paws can sit over the edge.

For a peek behind orange, move `gopher-depth` before the orange card and the hands after orange but before green. For a peek behind blue, move it before blue and hands after blue but before orange. The demo also scales and positions the character to fit each exposed edge; these are staged poses, not a physically continuous walk through depth.

The fixed `holdout-front`, `holdout-middle` and `holdout-back` masks hide the gopher behind the relevant card silhouettes. The front holdout deliberately also hides the gopher through the open r: the r remains transparent to the background. This is a graphic logo treatment, not physically literal occlusion. Remove or replace the mask if you want to see the character through the r. Keep the mask on the stationary depth wrapper and animate its child.

The rear card strips preserve the negative-space gaps; their previously hidden top ends are reconstructed so the cards remain coherent when the gopher moves away. Completed rear-card variants reveal full surfaces, so they change the static logo if you simply swap them in. Use those variants when the cards move apart or add suitable gap masks. Treat reconstructed surfaces as working animation assets, not new approved icon masters.

Each holdout also masks the area below its card, preventing the character from emerging under the logo during a downward hide. Remove that lower holdout if an animation should deliberately emerge from the bottom.

For hide, retract the paws first, then lower the character behind the stationary mask. For dance, bring the gopher and paws in front of the stack; animate the root bounce and rotation, arms, paws and feet about the pivots in `rig.json`. The demo completes a finite dance and respects reduced-motion preferences.

## Integration notes

Use the SVG inline to target groups; an `<img>` does not expose its internal elements to the page. Scope selectors to the component and prefix SVG IDs and mask references per instance. The preview has one instance and uses fixed IDs. The existing React app already declares `motion`; these groups can be animated with that dependency or ordinary CSS/JavaScript.

Give one accessible name to the whole mark; hide decorative parts from assistive technology. Keep a static pose for reduced motion. Avoid perpetually dancing in navigation: the demo is intentionally user-triggered. Do not replace launcher/tray artwork with the mascot without an intentional brand decision.

Creator credit: Go gopher by Renee French, licensed under CC BY 4.0. Adapted for Rolle; this kit adds a vector reconstruction, hidden anatomy and animation layers. Preserve `ATTRIBUTION.md` with redistributed artwork.
