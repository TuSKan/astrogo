// Package atlas decodes published light-pollution atlases (artificial zenith
// sky brightness, in mcd/m²) into a geographic
// [github.com/TuSKan/astrogo/skybrightness.SQMProvider], and composes them
// (plus [github.com/TuSKan/astrogo/skybrightness/lpmap]'s live API and the
// data-free Bortle/scalar fallbacks) into one easy entry point, [Resolver].
//
// # Just want a light-pollution floor?
//
//	result, err := atlas.FloorAt(ctx, site.Location(), atlas.WithLayer(atlas.LayerWorldAtlas))
//
// Pick a [Layer] and call [FloorAt] — that's the whole surface most
// callers need. ([NewResolver] + [Resolver.Floor] is the same thing with
// the atlas file held open across many queries; prefer it when resolving
// a whole list of sites.) [LayerAuto] (the default) tries the FRESHEST
// available source automatically — VIIRS (newest published year) before
// the 2015-frozen World Atlas — and reports what it tried and why an
// earlier choice didn't answer (see [Result.Attempts]). Freshness is not
// the same as fidelity: see "Sources and fidelity order" below, and pick
// [LayerWorldAtlas] explicitly when the propagated model matters more than
// the decade of lighting change since 2015. A download-backed layer
// ([LayerWorldAtlas]/[LayerVIIRS]) downloads and extracts its archive
// automatically on first use (still consent-gated, see below) and logs its
// own progress by default (see [WithQuiet] to disable it) — no separate
// download/progress plumbing to wire up.
//
// The lower-level pieces are still exported and independently usable:
// [NewFalchiProvider]/[NewVIIRSProvider]/etc. decode a caller-supplied file
// with no download at all, and [EnsureWorldAtlas]/[OpenWorldAtlas]/
// [EnsureVIIRSAnnual]/[OpenVIIRSAnnual] handle exactly one source's
// download+extract+validate without the multi-layer fallback logic.
//
// This is a pure-Go, no-CGO sibling of the core skybrightness package and is
// NEVER imported by it (enforced by an import-graph test in the core
// package) — atlas is free to import its own siblings ([Resolver] imports
// [github.com/TuSKan/astrogo/skybrightness/lpmap] for [LayerLightPollutionMap]),
// just never the other way around. Every download in this package goes
// through the same remote-package consent gate every other bulk download in
// this library uses — nothing here bypasses it.
//
// # Quantity and conversion
//
// Atlases store the artificial-only zenith radiance in mcd/m². It is
// converted to a V-band surface brightness with the cited relation
// m = −2.5·log₁₀(L/1.08e8) (the natural zenith background 0.171168465 mcd/m²
// maps to 22.0 mag/arcsec²; see
// [github.com/TuSKan/astrogo/skybrightness.SurfaceBrightnessFromMcdM2]). The
// returned value is ARTIFICIAL ONLY — the natural background (airglow, zodiacal
// light, moonlight) is supplied by the model's other components, so do not
// double-count it by folding a fixed natural term into the provider.
//
// # Sources and fidelity order
//
// Fidelity (highest first): LPA ≈ WA (both propagated through Cinzano
// radiative transfer of VIIRS → artificial sky brightness) > VIIRS (raw
// radiance + empirical fit, NOT propagated). There is a trilemma —
// propagated / fresh / downloadable: pick two — so all three are offered.
//
//   - WA — Falchi et al. 2016, "The new world atlas of artificial night sky
//     brightness", Sci. Adv. 2, e1600377. Data ("World Atlas 2015", ~2.9 GB
//     Float32 GeoTIFF, 30″, mcd/m²): GFZ DOI 10.5880/GFZ.1.4.2016.001,
//     CC BY-NC 4.0 (non-commercial). The highest-fidelity floor this package can
//     reach (propagated + downloadable), but frozen at 2014/15 — which is why
//     [LayerAuto] tries VIIRS first and this second. [NewFalchiProvider] (windowed) / [LoadFalchiGrid]
//     (clipped tiles) both take a caller-supplied file; [EnsureWorldAtlas] /
//     [OpenWorldAtlas] additionally handle downloading and extracting the
//     archive itself, opt-in and consent-gated via remote.WorldAtlas.
//   - LPA — Lorenz "Light Pollution Atlas 2024", djlorenz.github.io/astronomy/lp/ —
//     freshest propagated atlas, same units, but not published as a clean
//     numeric grid today (see [NewLorenzProvider]).
//   - VIIRS — annual composites (VNP46A4/VJ146A4, EOG VNL), raw upward radiance
//     (nW·cm⁻²·sr⁻¹), 2012–2025, as GeoTIFF or NASA HDF5. Fresh + downloadable
//     but NOT propagated: [NewVIIRSProvider] (GeoTIFF) and [NewVIIRSHDF5Provider]
//     (HDF5) apply the Sánchez de Miguel et al. 2020 empirical radiance→SQM fit
//     (lower fidelity; correlation degrades at dark sites). DOI
//     10.1038/s41598-020-64673-2.
//   - Conversion constants: lightpollutionmap.info/help.html.
//
// # File formats
//
// GeoTIFF is read by a built-in pure-Go windowed reader (see [NewFalchiProvider]):
// classic (non-BigTIFF) single-band 32/64-bit float, striped or tiled,
// uncompressed / LZW / deflate, with the floating-point predictor. LZW is
// implemented here rather than via compress/lzw — TIFF's variant widens codes
// one step earlier, and the stdlib reader rejects real files outright.
// HDF5 (NASA Black Marble granules) is read via the pure-Go github.com/scigolib/hdf5
// library (no CGO) through [LoadHDF5Grid] / [NewVIIRSHDF5Provider]; the whole
// dataset is loaded, so use per-tile granules rather than the global mosaic.
//
// # Encoding support
//
// The GeoTIFF reader handles classic TIFF (little/big endian), 32/64-bit float
// samples, single band, uncompressed or deflate, no predictor, and strip or
// tile layouts. Unsupported encodings (LZW, predictors, integer samples) return
// [ErrUnsupportedTIFF]; convert with, e.g.,
// `gdal_translate -ot Float32 -co COMPRESS=DEFLATE -co PREDICTOR=1 in.tif out.tif`.
package atlas
