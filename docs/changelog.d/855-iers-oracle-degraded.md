---
type: Fixed
pr: 855
---
**`time`'s IERS oracle tests failed while the IERS timescale service refused
every request** with a firewall 400. They now skip when a control request that
cannot be wrong is refused too, and still fail when it is answered.
