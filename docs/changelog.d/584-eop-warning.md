---
type: Fixed
pr: 584
---
**Zero EOP no longer arrives in silence.** With nothing loaded, as in a program that never imported `remote/eop`, `Time.EOP`, the UT1↔UTC fallback and `Time.UT1` now emit the one-time EOP warning, with its cause. Before, they returned zero DUT1 and polar motion without it. `RegisterModel(ZeroModel{})` stays silent. `Time.UT1` still returns no error there, and its doc no longer promises one.
