---
type: Fixed
pr: 74
---
**A validation test was passing because of a bug, and started failing when the bug was fixed.** It rendered a TDB epoch with `Time.Format`, which since #50 correctly produces the *UTC* calendar string, then told Horizons to read it as TDB — a 69.18-second shift, applied to both the elements and the vectors compared against them. Eros's divergence read 4.1 arcsec against a 2.0 tolerance; sending the epoch as a Julian Date restores the 0.04-to-0.56 arcsec the test's own comment describes.
