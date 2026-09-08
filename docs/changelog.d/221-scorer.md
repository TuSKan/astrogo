---
type: Changed — BREAKING
pr: 221
---
`plan.ScoreObservable` no longer exists; scoring is `plan.Scorer{...}.Score(obj, t)`.
It took six parameters, two of them nilable pointers that were nil at nearly
every call site including the README's — and two adjacent nils of different
types can be transposed without the compiler noticing (#116).
