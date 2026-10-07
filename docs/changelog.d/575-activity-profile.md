---
type: Fixed
pr: 575
---
**`MeteorShower.ObservedRate` follows the date.** It applied the peak ZHR on every night of the year, predicting 97 Perseids an hour on 1 March. The new `MeteorShower.ZHRAt` gives the rate at a time: IMO's maximum shaped by a new `plan.ActivityProfile`, Jenniskens (1994)'s fit to each stream, and zero outside the activity window. A full-turn window, [0°, 360°], no longer collapses to a single instant.
