// Package natural implements the natural-sky components of Sky Brightness
// V2 (airglow, zodiacal light, integrated starlight, diffuse galactic
// light, scattered moonlight, twilight, aurora) plus the fast, simplified
// models — ConstantAirglow, KrisciunasSchaeferMoonlight — that carry
// astrogo v1's empirical V-band physics forward under the new spectral
// API. These are named for what makes them scientifically distinct (a
// constant floor with no wavelength/time dependence; the specific paper
// they implement), not for their vintage — see docs/skybrightness.md §15
// for why that distinction matters: every type here is a brand-new type
// implementing the new skybrightness.Component interface, not a wrapper
// around anything deleted from v1, and a future real spectral moonlight
// model (Jones et al. 2013) is a different algorithm, not a replacement
// for KrisciunasSchaeferMoonlight.
//
// Phase 1 ships only the fast components (constant_airglow.go,
// krisciunas_schaefer_moon.go); the real spectral airglow/zodiacal/moon/
// twilight/starlight/diffuse-galactic models land in Phase 2.
//
// This package is PURE: it may import skybrightness (core), coord,
// ephemeris, angle — never remote, fits, net/http, or the dataset/lpmap
// IO tier (docs/skybrightness.md §4, machine-enforced by
// skybrightness/importgraph_test.go).
package natural
