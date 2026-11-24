---
type: Fixed
pr: 1262
---
**Importing `plan` panicked on a host without a time-zone database** (Windows without Go installed, a minimal container): its known-site table loaded ten zones while the package initialized. `plan.NewKnownSite` now loads a site's zone on first use and returns the error; a program that must run there imports `time/tzdata`.
