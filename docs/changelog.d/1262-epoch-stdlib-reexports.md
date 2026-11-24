---
type: Removed
pr: 1262
---
Every re-export of the standard library was removed from the package, and `MustLocation` with them: `GoTime`, `GoDate`, `LocationUTC`, the `Duration`, `Location` and `Month` aliases, the duration, month and layout constants, `LoadLocation`, `Parse`, `ParseInLocation`, `Since`, `Until`, `FixedZone`, `Unix`, `Now`, `Sleep`, `After`, `NewTimer` and `NewTicker`. Use the standard library's `time` for each. `Time.GoTime`, a second name for `Time.ToGo`, was removed, and so was the `Weekday` type with its seven days: `Time.Weekday` returns `time.Weekday`.
