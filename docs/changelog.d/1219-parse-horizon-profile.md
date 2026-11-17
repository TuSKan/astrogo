---
type: Added
pr: 1219
---
**`plan.ParseHorizonProfile`** reads a horizon table, the azimuth/altitude pairs Stellarium's landscapes, APT and NINA write, into a `HorizonProfile` for `WithHorizonProfile`, interpolating across north and stepping where two points share an azimuth. A line it cannot read is `plan.ErrHorizonProfile`.
