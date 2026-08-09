// Package natural implements the natural-sky components of Sky Brightness
// V2 (airglow, zodiacal light, integrated starlight, diffuse galactic
// light, scattered moonlight, twilight, aurora) plus the fast, simplified
// models — ConstantAirglow, VBandMoonlight — that carry astrogo v1's
// empirical V-band physics forward under the new spectral API. Both are
// named consistently for what makes them scientifically distinct — a
// constant floor with no wavelength/time dependence; a broadband V-band
// fit, not a spectral one — not for their vintage and not for which paper
// they cite (that citation lives in each type's Algorithm/AlgorithmRef,
// e.g. VBandMoonlight's implements Krisciunas & Schaefer 1991, PASP 103,
// 1033). See docs/skybrightness.md §15 for why that distinction matters:
// every type here is a brand-new type implementing the new
// skybrightness.Component interface, not a wrapper around anything
// deleted from v1, and a future real spectral moonlight model (e.g. Jones
// et al. 2013) is a different, structurally distinct algorithm — not a
// replacement for VBandMoonlight, and not blocked from claiming its own
// plain name by this one.
//
// Phase 1 ships only the fast components (constant_airglow.go,
// vband_moonlight.go); the real spectral airglow/zodiacal/moon/twilight/
// starlight/diffuse-galactic models land in Phase 2.
//
// This package is PURE: it may import skybrightness (core), coord,
// ephemeris, angle — never remote, fits, net/http, or the dataset/lpmap
// IO tier (docs/skybrightness.md §4, machine-enforced by
// skybrightness/importgraph_test.go).
package natural
