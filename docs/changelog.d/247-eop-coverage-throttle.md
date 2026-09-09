---
type: Fixed
pr: 247
---
An EOP lookup outside the IERS bulletin's coverage no longer re-reads and
re-parses the whole file on every call — 71 ns for a covered epoch against
11 ms and 15.6 MB for one just outside. Scheduling more than a year ahead and
historical work before 1973 were both on that path (#247).
