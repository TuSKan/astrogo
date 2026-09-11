---
type: Changed — BREAKING
pr: 274
---
**`remote.APIClient` is gone; `remote.Client` carries the request methods.** `Get`, `GetJSON`,
`PostForm` and `PostJSON` are methods on the policy that decides whether the request may happen
at all, so a caller holds one object instead of two that each answered half the question.
`remote.Default()` is the process-wide one; a component wanting its own timeout, pacing or token
takes `remote.Default().Clone()` and calls `SetAPIOptions`. Transports are built per endpoint with
that endpoint's registered `Timeout`, which fixes a latent defect: one client used for two
endpoints previously applied whichever timeout was named at construction to both.
