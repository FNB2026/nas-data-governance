# NDG app icon

The canonical Wails raster asset is `cmd/ndg-desktop/build/appicon.png`, generated deterministically from `cmd/ndg-desktop/build/appicon.svg`. It shows the exact uppercase letters **NDG** in white on a flat brand-blue `#0071e3` rounded tile, transparent outside the tile. This is a source-asset update for future builds; the published v0.5.0-beta.11 Tag, DMG and signatures are immutable and unchanged.

## Source of truth

`appicon.svg` is 1254×1254, tile corner radius 330 (26.3%, the same ratio as the website header mark), lettering `Helvetica Neue` bold 350px centred on the canvas. The PNG is rendered from that SVG at 1:1 with no image model involved, so the artwork can be reproduced or corrected by editing two numbers.

Derived copies share the same master and must not be redrawn separately:

- `cmd/ndg-desktop/frontend/src/web/ndg-icon.png` (64×64, in-app web favicon)
- `packaging/fnos/ndg-readonly-test/app/ui/images/icon_256.png` and `icon_64.png` (fnOS research package)

## History

The 2026-10-12 asset was an image-model rendering with a gradient tile and a soft shadow. On 2026-11-20 it was superseded so that the app icon, the repository README and the official website all show the same flat blue `NDG` mark. The gradient version is no longer referenced anywhere and is not tracked.

Validation: alpha and 1254×1254 dimensions inspected; lettering checked for exact spacing and optical centring (white bounding box centred at 627,627 on the 1254 canvas); 112px and 64px downscales checked for legibility. Wails packaging and the extracted bundle icns are checked separately on the build PR. This does not constitute a new signed release or App Store submission.
