---
type: Fixed
pr: 857
---
**Earth orientation was never downloaded while the IERS data center refused
every request** with a firewall 400, so every epoch used zero DUT1 and polar
motion. `remote.IERSFinals2000A` now fetches finals2000A from its producer, the
IERS Rapid Service/Prediction Center at USNO.
