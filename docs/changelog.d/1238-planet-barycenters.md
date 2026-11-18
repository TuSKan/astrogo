---
type: Changed — BREAKING
pr: 1238
---
**Every `plan` planet is its system barycenter, NAIF 1 to 9:** `plan.NewMercury` and `plan.NewVenus` track NAIF 1 and 2, as `plan.NewMars` through `plan.NewPluto` already tracked theirs, and catalog targets naming Mercury or Venus map to the same points. **`plan.NewEarth` was removed:** Earth's barycenter is the Earth–Moon barycenter, and an astrogo observer stands on Earth.
