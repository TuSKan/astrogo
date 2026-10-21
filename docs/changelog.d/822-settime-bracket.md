---
type: Changed
pr: 822
---
**`coord.Context.SetTime` interpolates the precession-nutation state across a
day instead of rebuilding it every hour**, and places the observer at the new
instant. A day of altitudes at 15-minute ticks takes 0.48 ms where it took
3.4, and a moved Context is within 0.007″ of a fresh one where it drifted to
0.07″.
