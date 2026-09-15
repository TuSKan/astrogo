---
type: Added
pr: 297
---
`coord.Context.ICRSToITRS` and `ITRSToICRS` expose the rotation between the
celestial frame and the rotating Earth — station coordinates, ground tracks,
anything Earth-fixed. The matrix was already built and cached for every horizon
transform, so this is a matrix multiply rather than a second implementation,
and a test now pins the cached factored form as bit-identical to SOFA's
one-call `C2t06a` (#126).
