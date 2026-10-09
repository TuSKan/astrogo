---
type: Changed — BREAKING
pr: 699
---
**`time.DeltaT` and `time.DeltaTUncertainty` take a `time.Time`, and ΔT is one value everywhere**: the Espenak & Meeus model before 1960, the IERS bulletin's measured value where it covers, and that value held past its end, where the conversions used to pin UT1 to UTC. `DeltaT` followed the model at every epoch, 6.2 s from the conversions in 2026. Past the bulletin, `Time.UT1` answers instead of failing, and the uncertainty grows from the bulletin's end (#696).
