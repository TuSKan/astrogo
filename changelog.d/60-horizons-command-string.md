---
type: Changed
pr: 60
---
**The Horizons query helpers take the COMMAND payload as a string** rather than a NAIF ID as an int, because a small body needs Horizons' designation syntax (`433;`) and an int cannot express it. `StateVector.NaifID`, which was written and never read, becomes `StateVector.Command` and records what was actually queried.
