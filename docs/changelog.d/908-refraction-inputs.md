---
type: Fixed
pr: 908
---
**A refraction whose fields are not air is refused, not clamped by SOFA into a plausible number.** The new `Refraction.Validate`
backs `plan.NewSite` and `Builder.Build`. Paths with no error to return, `coord.Context` among them, now give NaN. A NaN temperature
used to refract as −150 °C, 2.35 times too much.
