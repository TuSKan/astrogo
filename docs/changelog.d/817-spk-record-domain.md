---
type: Fixed
pr: 817
---
**`spk.EvaluateSegment` evaluated a type 2/3 segment's last record for a whole
record interval past its end.** The polynomial ran at τ up to 3, forty times the
true value a record out, with no error; an epoch outside its record's interval
is now refused. `jpl.Provider` never asked for one.
