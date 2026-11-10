---
type: Fixed
pr: 1028
---
**SIMBAD's network tests skip an outage instead of failing on it.** A test that resolved an unknown name had also been passing on an outage. Like VizieR's tests, they now check a request SIMBAD cannot reject before blaming astrogo for its error, and the unknown-name test requires "not found" rather than any error.
