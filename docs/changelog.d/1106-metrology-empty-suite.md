---
type: Fixed
pr: 1106
---
**A validation suite that measured nothing fails instead of reporting `VERIFIED`**: nothing exceeds a contract when
there is nothing to compare, so a corpus that loaded empty read as the best result a suite can have. It is now
recorded as not verified.
