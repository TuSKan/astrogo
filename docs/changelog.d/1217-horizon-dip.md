---
type: Changed — BREAKING
pr: 1217
---
**A site's height no longer dips its horizon.** Rise and set subtracted a sea horizon's dip for the height above the ellipsoid, so Denver rose the Sun 7 min before USNO. The dip is now zero unless the new `plan.WithHorizonHeight` says how far the observer looks down on the visible horizon; `WithHorizonHeight(siteHeight)` restores the old times.
