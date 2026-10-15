---
type: Added
pr: 783
---
**`resolve.BrightRequest.At` dates a bright search.** `catalog/sbdb` bounds each comet by where its orbit has it around that instant rather than by its perihelion alone, so a comet decades from perihelion is no longer a candidate, and `plan.VisibleTonight` dates its search in the middle of the night it plans. The zero time keeps the undated search.
