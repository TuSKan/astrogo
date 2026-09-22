---
type: Fixed
pr: 369
---
**37 tests that skipped on any error, and so could never fail, now identify it
first.** Each named a cause the code had not established — "SkyCalc did not
answer", "(network issue?)" — and three were not about a service at all: one
skipped when IMCCE's document failed to decode into a struct declared in this
repository, so a schema change would have read green indefinitely. A new
`docsguard` check fails any skip that is the first statement inside a bare
`err != nil` guard, with an allowlist for genuine environment preconditions.
