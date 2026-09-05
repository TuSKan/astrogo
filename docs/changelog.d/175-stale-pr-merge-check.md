---
type: Added
pr: 175
---
**CI now re-checks every open pull request when `main` moves.** A PR's own
checks test it merged into its base, but nothing re-runs them when the base
advances — so a PR can sit green while the branch it would merge into changes
underneath it. #169 did exactly that: it merged with no textual conflict and
did not compile, because #165 had meanwhile rewritten the function it touched.
The new job trial-merges each open PR into the pushed commit and builds the
result (#169).
