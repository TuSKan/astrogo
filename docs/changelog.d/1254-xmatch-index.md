---
type: Changed
pr: 1254
---
**`xmatch.Match` compares only the rows that lie near each other**, found through a declination index, with the same result: two all-sky catalogs of 2,000 rows each match in 8 ms rather than 1.72 s, and of 100,000 rows each in 0.6 s.
