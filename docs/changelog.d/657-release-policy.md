---
type: Added
pr: 657
---
**A release every week, and a GitHub Release for every tag.** `docs/RELEASING.md` sets the cadence (every Monday when anything merged; security fixes and regressions within a day), reads the version off the changelog fragments, and routes the release commit through a pull request. A tag push now publishes a GitHub Release with that version's changelog section as its notes.
