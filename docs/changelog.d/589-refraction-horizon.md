---
type: Fixed
pr: 589
---
**The default refraction now reaches the horizon.** `atmosphere.RefractionSOFA`, which every `Site` uses, was SOFA's series clamped at 2.87°: 10.3′ on the horizon against the almanacs' 34′. It hands over to Bennett-NA below 10°, within 13″ of Hohenkerk & Sinclair's ray tracing on the horizon, and is unchanged above. A line of sight more than about 4° below the horizon is no longer refracted.
