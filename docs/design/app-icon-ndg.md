# NDG app icon

The canonical Wails raster asset is `cmd/ndg-desktop/build/appicon.png`. It replaces the framework's default W with the exact uppercase letters **NDG**: white lettering on a blue rounded tile, transparent outside the tile. This is a source-asset update for future builds; existing v0.5.0-beta.11 Tag, DMG and signatures are immutable and unchanged.

Created with the built-in image_gen tool on 2026-10-12 at the user's request. No customer data or third-party reference artwork was used. Original generated PNG is retained locally. The committed 1254×1254 RGBA source is square; pinned Wails v2.13's standard icns encoder generates the 1024/512/256/128/64/32 variants from it. No Python image manipulation or release asset replacement was used.

Prompt: “Production macOS desktop app icon for NDG, a local NAS data governance workbench. Transparent square canvas outside a centered blue rounded-square tile, restrained gentle gradient, subtle shadow. Center exactly the uppercase letters NDG (N D G), bold white geometric sans-serif on one line, readable at small sizes. No W, extra marks, illustration, small text or watermark.”

Validation: source alpha/dimensions inspected; generated artwork visually checked for exact lettering; Wails desktop packaging and the extracted bundle icns checked separately. Build/check results are recorded on the PR. This does not constitute a new signed release or App Store submission.
