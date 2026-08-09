// Package natural implements the natural-sky components of Sky Brightness
// V2 (airglow, zodiacal light, integrated starlight, diffuse galactic
// light, scattered moonlight, twilight, aurora) plus the Legacy* fast
// models that carry astrogo v1's empirical V-band physics forward under
// the new spectral API — see docs/skybrightness.md §15 for why Legacy is
// a physics-vintage label, not a backward-compatibility shim: every
// Legacy* type here is a brand-new type implementing the new
// skybrightness.Component interface, not a wrapper around anything
// deleted from v1.
//
// Phase 1 ships only the Legacy* components (legacy_airglow.go,
// legacy_moon.go); the real spectral airglow/zodiacal/moon/twilight/
// starlight/diffuse-galactic models land in Phase 2.
//
// This package is PURE: it may import skybrightness (core), coord,
// ephemeris, angle — never remote, fits, net/http, or the dataset/lpmap
// IO tier (docs/skybrightness.md §4, machine-enforced by
// skybrightness/importgraph_test.go).
package natural
