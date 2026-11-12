---
type: Changed
pr: 1095
---
**`catalog/mast`'s tests now cover every reachable path**, up from 83.6% to 95.5%: the first of several matches, a
name MAST does not know, MAST's own error status, a repeated name served from the cache, an iterator stopped early, and
a truncated answer reported as `io.ErrUnexpectedEOF`.
