---
type: Changed — BREAKING
pr: 216
---
`skybrightness/plan.Spec.Sky` is now the four-method `plan.Sky` interface rather
than `*dataset.Sky`. A `*dataset.Sky` satisfies it, so every caller is unchanged
— what it buys is that `LimitingMagnitudeAt` can be evaluated without a network
and 145 MB of reference data, which is why it went uncovered on every commit
(#122).
