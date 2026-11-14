---
type: Fixed
pr: 1175
---
`coord.Context` passes a star's parallax to SOFA in arcseconds, as SOFA expects, so a star's observed place includes its annual parallax and the perspective term: Barnard's Star moved 0.78″ and α Cen 0.80″. They were dropped since the transform was written.
