---
type: Added
pr: 258
---
`core.ID` and `jpl.NAIFFor` now document which bodies are system barycentres
rather than planets — the giant planets are, and the gap is 0.03–0.05″ against
a reference that defaults to the body centre. A test pins the numbers and the
centre-versus-barycentre split (#258).
