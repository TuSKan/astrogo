---
type: Added
pr: 265
---
Offline tests for the apparent-place failure paths #263 added: each of the four
provider fetches is made to fail on its own and asserted to report *which* one
did, and the three degenerate deflection geometries are pinned as returning the
place undeflected rather than NaN (#265).
