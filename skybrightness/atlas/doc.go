// Package atlas decodes published light-pollution atlases (artificial zenith
// sky brightness, in mcd/m²) into a geographic
// [github.com/TuSKan/astrogo/skybrightness.SQMProvider].
//
// It is a pure-Go, no-CGO sibling of the core skybrightness package and is NEVER
// imported by it (enforced by an import-graph test in the core package). Loaders
// perform no runtime downloads: the caller supplies the data file as an
// io.ReaderAt; this file lists the source URLs/DOIs.
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
//     Float32 GeoTIFF, 30″, mcd/m²): GFZ DOI 10.5880/GFZ.1.4.2016.001. Default
//     floor (propagated + downloadable, frozen 2014/15). [NewFalchiProvider]
//     (windowed) / [LoadFalchiGrid] (clipped tiles).
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
// GeoTIFF is read by a built-in pure-Go windowed reader (see [NewFalchiProvider]).
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
