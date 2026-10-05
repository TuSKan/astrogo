---
type: Changed
pr: 493
---
**`plan`'s test suite runs its 26 slowest serial tests in parallel**, cutting
the package's race-detector run from 261 s to 174 s locally, against CI's
600 s timeout (#490).
