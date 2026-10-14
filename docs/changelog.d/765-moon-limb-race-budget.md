---
type: Changed
pr: 765
---
**`plan`'s moon-limb test covers two months under `-short`**, the race-detector job's flag, rather than a year: it was 151 s of that job's 600 s budget on an idle machine and the test CI timed out in, and is now 24.6 s. The Lint and Test jobs still run the full year.
