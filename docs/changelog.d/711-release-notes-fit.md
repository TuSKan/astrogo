---
type: Fixed
pr: 711
---
**A release whose notes exceed GitHub's 125,000-character limit now publishes** instead of failing at creation, as v0.20.0's did at 165,662. Whole sections are kept in priority order, Security and breaking changes first, and any section left out points to `CHANGELOG.md` at the tag.
