---
type: Added
pr: 703
---
`plan.EventSpec.UpperLimb` applies `Threshold` to a moving body's upper limb, raising its altitude by half its `AngularDiameter` at every instant the solver evaluates; `Validate` returns `plan.ErrUpperLimbNeedsBody` for a target that is not a `MovingBody`.
