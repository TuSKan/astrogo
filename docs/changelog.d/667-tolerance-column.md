---
type: Fixed
pr: 667
---
**The README said the natural sky was validated to 0.05 mag against GAMBONS**, and `VALIDATION.md` gave three rows a tolerance that nothing asserts: 0.05 mag beside a suite held to 1 mag, 1e-7 deg beside Horizons at 3 arcsec, and 1e-12 d beside round trips at 1e-6 s and 5 s. Each row now states the bound its suites assert, the README states what is measured (up to 0.28 mag per altitude band with airglow included), and `internal/docsguard` holds every row to its cited suite's contract.
