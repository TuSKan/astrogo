---
type: Added
pr: 276
---
**`time.TCG` and `time.TCB` — the two coordinate time scales.** TCG is the geocentric
frame's coordinate time (TT rescaled by L_G, 22 ms/year) and TCB the barycentric frame's
(TDB rescaled by L_B, 0.49 s/year), which is what relativistic geodesy, orbit integration
and pulsar timing are quoted in. Both convert to and from every other scale and are exact
in both directions; verified against SOFA's own published values for `iauTttcg`,
`iauTcgtt`, `iauTdbtcb` and `iauTcbtdb` (#126).
