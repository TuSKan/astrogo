---
type: Fixed
pr: 755
---
**`vector.Vec3.ToSpherical` keeps its precision at the poles.** It took latitude from `asin(Z/|v|)`, which lost up to 2.7 mas within 0.2″ of a pole, in every `coord` frame's `FromUnitVector`; it now uses `atan2` as SOFA does, and `ephemeris.ToICRS` and a moving body's `plan.TargetDetails` use it in place of their own copies. A longitude a hair below zero no longer wraps to exactly 2π.
