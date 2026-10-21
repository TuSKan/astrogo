---
type: Fixed
pr: 830
---
**The test holding `SatellitePasses`' moving Context to a fresh one could not
fail**: since #812 its route cancels the Context's error (0.0000″ against a
pre-#822 0.1″). It now holds that route to 0.01 mas, and a route where the error
shows to SetTime's ≲0.007″ times the satellite's parallax amplification.
