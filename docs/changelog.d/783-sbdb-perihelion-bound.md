---
type: Fixed
pr: 783
---
**`catalog/sbdb`'s `SearchBright` ranks its candidates among bodies that can reach the bound.** It kept the 50 lowest-H bodies under the bound plus a margin, and those were 47 trans-Neptunian objects that never get brighter than about 14, so `plan.VisibleTonight` never considered Juno, Hebe or Iris. It now excludes any body whose perihelion keeps it fainter than the bound at every point of its orbit.
