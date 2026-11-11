---
type: Fixed
pr: 1075
---
**Eight comments and docs no longer point at files that moved or were deleted**, and `CONTRIBUTING.md` lists the
module's five deliberate `init()` functions instead of claiming one. A new `docsguard` test holds every repository file
named in a Go comment or in Markdown, and every Markdown link target, to the tree.
