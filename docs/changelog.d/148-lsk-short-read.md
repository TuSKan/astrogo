---
type: Fixed
pr: 148
---
**A truncated leap-second kernel parsed into a short table.** `lsk.NewReader`
never checked `scanner.Err()`, so a read that failed part-way kept the entries
seen so far and returned successfully — the same shape as the dropped-2017-entry
bug, reached by a short download instead of a parsing slip.
