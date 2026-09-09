---
type: Added
pr: 260
---
`coord/sofareference_test.go` pins astrogo's topocentric reduction against
SOFA's `iauAtco13` over 210 site/direction/epoch combinations, offline, at a
measured maximum of 0.000″ against a 1 µas contract. It is what excludes the
last stage of the observed-place pipeline as the source of the ~0.5″ azimuth
residual (#260).
