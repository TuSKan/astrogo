---
type: Added
pr: 279
---
`coord.SkyOffset` is a frame centred on a target, so positions near it can be
given as offsets — dither and mosaic patterns, offset guide stars, slit
layouts, finder charts. It is a rotation of the sphere rather than a projection
onto a plane, which is the difference between it and subtracting coordinates: a
point one degree due east of a target at δ = 80° differs from it by 5.7° of
right ascension and by three arcminutes of declination (#126).
