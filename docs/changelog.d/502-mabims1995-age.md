---
type: Fixed
pr: 502
---
**`CrescentParams.MABIMS1995` dropped the criterion's 8-hour age alternative**
("2-3-8"). It now reads the new `Age` field, and with `Age` left zero it
behaves as before (#496).
