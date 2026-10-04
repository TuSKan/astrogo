---
type: Fixed
pr: 428
---
**`plan.MoonPhases` samples to the end of its window**, so a phase — and an
eclipse at that syzygy, through `LunarEclipses` and `SolarEclipses` — in the
window's last partial six-hour step is no longer lost, and a refinement that
fails is returned instead of silently dropping the phase (#419).
