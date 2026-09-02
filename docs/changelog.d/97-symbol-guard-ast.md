---
type: Fixed
pr: 97
---
**The documentation guards matched symbols with a regular expression**, which
could not tell a method from a same-named function, nor `plan` from
`skybrightness/plan`. They now read the module with Go's own parser, and both
guards share one index. Two more stale citations found and fixed.
