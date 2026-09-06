---
type: Fixed
pr: 184
---
**`time` re-exported fifteen standard-library functions as reassignable package-level
`var`s, and its six layout strings as a `var` block.** Any package in the import graph
could reassign `time.Parse`, `time.Now` or `time.RFC3339` process-wide, in the package
every epoch calculation goes through. They are functions and constants now — identical
at every call site, so nothing outside had to change (#113).
