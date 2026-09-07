---
type: Added
pr: 213
---
CI runs `apidiff` between a pull request's head and its base, and fails when the
exported API changes incompatibly with no fragment declaring it. Nineteen minor
releases in eight weeks is fine pre-1.0 only if the breaking diff is
machine-reported rather than found by a downstream build (#121).
