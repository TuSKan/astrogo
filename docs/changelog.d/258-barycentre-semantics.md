---
type: Added
pr: 258
---
`core.ID` and `jpl.NAIFFor` now document which bodies are system barycenters
rather than planets — the giant planets are, and the gap is 0.03–0.05″ against
a reference that defaults to the body center. A test pins the numbers and the
center-versus-barycenter split (#258).
