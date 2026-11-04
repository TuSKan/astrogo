---
type: Fixed
pr: 978
---
**CLAUDE.md and CI's comments say which test files carry which build tag, and where each tag runs.** CLAUDE.md filed `plan`'s USNO, NASA eclipse and AstroPixels comparisons under `validation`, though all three are `integration`. A CI comment counted 59 tagged test files, against 93 on `main`, and said they ran only weekly, though `network` runs nightly and `integration` also runs on every pull request.
