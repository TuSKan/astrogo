---
type: Fixed
pr: 84
---
**Roadmap item 39 said "Not Started" with all five boxes ticked** — it shipped
in #74. `internal/docsguard` now checks every item's status against its own
checkboxes, in both directions; a box beginning "Optional" is excluded so a
deliberately untaken extension does not force an item out of Done.
