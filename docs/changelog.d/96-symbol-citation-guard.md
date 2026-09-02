---
type: Fixed
pr: 96
---
**Documents named functions that no longer exist.** `internal/docsguard` now
checks every backticked, package-qualified symbol in every document against
the code — 174 of them — and `declaredIn` learned to see entries inside
grouped `const`/`var`/`type` blocks, which it had been blind to.
