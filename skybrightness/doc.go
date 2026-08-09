// Package skybrightness is astrogo's spectral, all-sky, observatory-grade
// sky-brightness engine (Sky Brightness V2). It predicts ground-observed
// spectral sky radiance
//
//	L_lambda(lambda, altitude, azimuth, site, epoch)   W*m^-2*sr^-1*nm^-1
//
// for arbitrary terrestrial sites, arbitrary horizontal directions, point
// or all-sky queries, spectral or passband-integrated output, decomposed
// into natural and anthropogenic components, under clear, partly-cloudy,
// or overcast skies, in climatological, historical, nowcast, or forecast
// atmospheric states, with uncertainty and full provenance attached to
// every result. See docs/skybrightness.md for the full design document —
// this comment is a summary, not a substitute for it.
//
// # No backward compatibility
//
// This is a complete, ground-up replacement of astrogo v1's skybrightness
// package. There is no shim, no adapter, and no deprecation cycle — v1's
// Model, Component (old shape), SurfaceBrightnessV, Nanolambert,
// SQMProvider, Floor, CompositeModel, RadianceToArtificialSB, and
// atlas.Resolver are all deleted, not renamed. See
// docs/skybrightness.md §16 for a symbol-by-symbol migration table.
//
// # Six quantities this package never conflates
//
// Satellite upward radiance, V-band surface brightness, an SQM reading,
// luminance, horizontal irradiance, and limiting magnitude are six
// distinct physical quantities. This package never treats any pair of
// them as interchangeable; every conversion between them is an explicit,
// named, unit-tested function (see units.go and passband.go). In
// particular, a raw VIIRS-DNB pixel is never converted directly to a sky
// brightness via a single empirical fit — see the Artificial component's
// documentation (skybrightness/artificial, from Phase 4) for why: one
// ~15-arcsec satellite pixel measures upward radiance from one small patch
// of ground, while zenith skyglow is an additive flux integral over
// scattered light from sources up to ~300 km away.
//
// # Linear flux space
//
// Every Component computes a contribution to L_total in LINEAR spectral
// radiance space. Surface brightnesses are logarithmic (mag/arcsec^2);
// summing them instead of the underlying radiances is a correctness bug.
// The engine sums components into Result.Total before any magnitude
// conversion happens, and Result.Components retains each component's own
// linear-space contribution.
//
// # Package layout
//
// This package (core) is pure: types, interfaces, the composite engine,
// and derived-output functions — no I/O, no network, no heavy
// dependencies. Real physics lives in siblings (skybrightness/natural,
// skybrightness/atmos, skybrightness/artificial, skybrightness/rt,
// skybrightness/surrogate, skybrightness/calib, landing across Phases
// 2-7), and real datasets live under skybrightness/dataset/... — the only
// tier allowed to import remote, fits, net/http, or
// github.com/scigolib/hdf5. plan imports this core package only; an
// Engine is assembled by the application (an example's main, or a
// caller's own setup) and injected into plan, not constructed inside it.
// importgraph_test.go machine-enforces this shape.
//
// # Modes and fallback
//
// A Request always names a Mode (Climatology, Historical, Nowcast,
// Forecast, UserSupplied, or Fast). Modes never fall back into one
// another silently: EvaluationOptions.Fallback defaults to
// FallbackForbidden, and any fallback that does occur under an explicit
// opt-in is recorded in Provenance.Fallbacks.
//
// # Accuracy, honestly stated
//
// This package's Phase 1 foundation proves unit correctness, passband
// integration correctness, linear-space additivity, determinism, and
// allocation behavior — it proves nothing about physical accuracy. This
// repository holds no ground-truth SQM/TESS field measurements today, so
// the accuracy targets in docs/skybrightness.md §12 are engineering
// goals, not achieved claims, until real observational data exists to
// validate against.
package skybrightness
