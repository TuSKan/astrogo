---
type: Changed — BREAKING
pr: 576
---
**A star rises and sets on the almanac horizon.** `Site.RiseSetThreshold` is −(34′ + dip), not the dip alone, so `VisibilityEvents`, `DayEvents`, `Episode`, `IsCircumpolar` and `IsNeverUp` agree with USNO and Skyfield, which put a star's horizon 34′ down; rises were 2.4 to 7 minutes late. `plan.WithRefraction` no longer exists, since `IsCircumpolar`'s default now includes the refraction it added; `WithHorizonAltitude(0)` gives the geometric horizon.
