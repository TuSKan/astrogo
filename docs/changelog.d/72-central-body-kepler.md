---
type: Added
pr: 72
---
**Central-body two-body propagation in `ephemeris/kepler`.** `Elements.WithCentralBody` refers an element set to a planet instead of the Sun, `CentralBodyFor` supplies the parent's *body* mass parameter so the 12%-wrong system value cannot be picked by mistake, and the provider composes a satellite through its parent. Verified against Kepler's third law — 421,800 km about Jupiter gives 1.7699 days, Io's sidereal period, from a distance and a mass parameter alone. This is the machinery and the constants, not a satellite ephemeris: Laplace-plane frames and J₂ precession are still open.
