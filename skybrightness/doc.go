// Package skybrightness models night-sky surface brightness as a sum of
// additive physical components evaluated per pointing and per time, and derives
// observability quantities (such as a limiting magnitude) from the total.
//
// The design separates the brightness MODEL from the observability CONSTRAINT.
// A [Model] returns the total sky surface brightness toward a horizontal
// pointing at a given epoch; a limiting-magnitude conversion and the
// scheduling constraint that consumes it live in the plan package. This lets a
// caller run a cheap "light-pollution floor only" model or compose the floor
// with scattered moonlight, zodiacal light, and airglow purely by adding
// components — without changing the constraint.
//
// # Linear flux space
//
// Surface brightnesses are LOGARITHMIC (mag/arcsec²). Component contributions
// MUST therefore be summed as linear radiances ([Nanolambert]) and converted
// back to a [SurfaceBrightnessV] only at the boundary. Summing magnitudes is a
// correctness bug; see [Nanolambert].
//
// # Accuracy and scope
//
// This is observatory-grade-*lite*, not a full radiative-transfer sky model.
// The scattered-moonlight component (Krisciunas & Schaefer 1991) is accurate to
// ~8–23% away from full Moon. The model targets true night; the twilight
// scattered-sunlight regime is out of scope (gate it with the existing Sun /
// AtNight twilight logic). No data tables are downloaded at runtime — spatial
// light-pollution grids must be supplied by the caller.
//
// # References
//
//   - Falchi et al. 2016, "The new world atlas of artificial night sky
//     brightness", Sci. Adv. 2, e1600377 — artificial-skyglow floor.
//   - Krisciunas & Schaefer 1991, PASP 103, 1033, "A model of the brightness of
//     moonlight" — scattered moonlight (V-band, closed form).
//   - Leinert et al. 1998, A&AS 127, 1 — zodiacal-light prescription.
//   - Noll et al. 2012, A&A 543, A92; Patat 2008, A&A 481, 575 — Cerro Paranal
//     sky-model component decomposition and airglow (reference for the dark-sky
//     floor value).
package skybrightness
