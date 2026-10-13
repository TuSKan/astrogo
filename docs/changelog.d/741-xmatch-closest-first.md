---
type: Fixed
pr: 741
---
**`catalog/xmatch.Match` pairs by position closest first, whatever the input order.** Each item used to take its nearest partner not yet taken, so an earlier row could claim a match 1.5″ away from one 0.1″ from a later row, and a row duplicated in one slice lost its positional match altogether.
