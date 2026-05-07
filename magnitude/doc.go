// Package magnitude computes apparent visual magnitudes for Solar System bodies,
// stars, asteroids, comets, and artificial satellites.
//
// The planet magnitude models follow Mallama & Hilton (2018), "Computing Apparent
// Planetary Magnitudes for The Astronomical Almanac", Astronomy & Computing 25,
// pp. 10–24. These are the algorithms adopted by the U.S. Naval Observatory and
// HMNAO for The Astronomical Almanac since the 2020 edition.
//
// Asteroid magnitudes support three IAU phase-curve models:
//   - H,G (Bowell et al. 1989) — legacy, used by MPC with G=0.15
//   - H,G₁,G₂ (Muinonen et al. 2010) — current IAU standard
//   - H,G₁₂* (Penttilä et al. 2016) — for sparse data
//
// Comet magnitudes use the IAU standard total-magnitude model (M₁/k₁).
//
// Satellite magnitudes use the McCants/Quicksat or Molczan standard magnitude
// convention with sphere or cylinder phase functions.
//
// Star magnitudes apply atmospheric extinction via Bouguer's law using the
// Pickering (2002) airmass formula from the atmosphere package.
package magnitude
