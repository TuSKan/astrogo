---
type: Changed — BREAKING
pr: 221
---
`Observable.GetDetails` takes a `plan.DetailOverrides` struct instead of
`props ...string` read two at a time. That form had three silent failures: an
odd count dropped the last argument, a misspelled key landed in `ExtraProps`
while the field it meant to override kept its computed value, and a key and
value could be swapped with nothing to notice (#116).
