---
type: Fixed
pr: 1009
---
**Four catalog tests name a failed request instead of reporting an empty target.** They read a `resolve` iterator and dropped the error it yields beside a zero target, so a timeout surfaced as a target with every field empty. Each now fails on the error itself.
