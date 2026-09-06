---
type: Added
pr: 194
---
Offline tests for `skybrightness/dataset/solar`'s CALSPEC parse path, taking it from 22.9%
to 71.4% — the unit conversion (Å→nm, erg s⁻¹ cm⁻² Å⁻¹→W m⁻² nm⁻¹, derived independently),
the row filters, the negative-flux clamp, both float widths, and the padded column names a
real CALSPEC file actually carries (#122).
