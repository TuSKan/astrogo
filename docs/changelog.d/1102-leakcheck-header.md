---
type: Fixed
pr: 1102
---
**The leak check refuses a profile header it cannot read instead of reporting no leaks**: a header without a count read
as zero, so a change to the runtime's wording would have switched the guard off in every package without a sound.
