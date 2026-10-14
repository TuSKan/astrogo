---
type: Fixed
pr: 757
---
**`lsk.UTCToET` holds the old leap-second count until the step.** It looked the count up 69.184 s after the UTC date, as if the kernel's midnight-UTC table were in TDB, so for the last ~69 s of every day ending in a leap second ET came out a second late, and every `jpl.Provider` position with it.
