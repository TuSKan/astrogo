---
type: Added
pr: 334
---
`coord.Galactocentric` and `coord.GalactocentricFrame` express a position in
the right-handed Cartesian frame centered on the Galactic center, in parsecs —
the frame a rotation curve, a disc scale height or a stellar stream is actually
written in. `GalactocentricFrame.FromICRS` takes the distance as an explicit
argument rather than reading `ICRS.Dist`, whose unit depends on the subsystem
that filled it in, and `coord.ParallaxDistance` converts a catalogue parallax
into the parsecs it wants. Measured parameters are arguments (R₀ = 8178 pc,
GRAVITY Collaboration 2019; z☉ = 20.8 pc, Bennett & Bovy 2019); the orientation
is not, so the axes come from the IAU Galactic frame this package already
implements rather than from a second definition that could drift from it.
Verified against Astropy's independent parameterisation of the same frame,
which astrogo never writes down: 0.33″ in the Galactic-center direction and
0.12″ in the roll, both of which are Astropy's rounding of the shared
convention. Positions only — astrogo has no space-velocity type, so a
Galactocentric *velocity* is not yet expressible. This was the last frame on
the [#126] checklist; the general transform graph on it remains open.
