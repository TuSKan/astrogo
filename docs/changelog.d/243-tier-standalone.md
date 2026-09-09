---
type: Fixed
pr: 243
---
`plan`'s `network` and `validation` test tiers run on their own again: the
`TestMain` registering the kernel backend and granting download consent was
gated to `integration`, so `go test -tags=network ./plan/` reported "this build
has no kernel backend" for 24 checks. A guard now requires a `TestMain` to
cover every tier its package has tests in (#243).
