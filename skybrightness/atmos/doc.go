// Package atmos implements the atmospheric-optics side of Sky Brightness
// V2: molecular (Rayleigh) transmission, aerosol optics, cloud optics,
// terrain/horizon screening. Phase 1 ships only an analytic,
// Rayleigh-only TransmissionModel so the engine is complete end-to-end;
// Phase 3 replaces it with the full Bodhaine et al. (1999) molecular
// treatment plus aerosol/cloud optics, behind the same
// skybrightness.TransmissionModel interface.
//
// This package is PURE: it may import skybrightness (core), atmosphere,
// coord — never remote, fits, net/http, or the dataset/lpmap IO tier
// (docs/skybrightness.md §4, machine-enforced by
// skybrightness/importgraph_test.go).
package atmos
