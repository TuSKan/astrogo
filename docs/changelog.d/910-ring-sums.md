---
type: Changed
pr: 910
---
**`skybrightness`'s hemispheric scattering sum applies each ring's weights and path factor once per ring, not once per sample**, in both the sky-map path and `ScatteredIn`. Reference sky maps run about 17% faster; results differ from before by at most about 6e-15 relative, so they are not bit-identical.
