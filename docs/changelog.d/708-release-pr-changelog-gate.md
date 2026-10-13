---
type: Changed
pr: 708
---
**A release pull request passes the changelog gate without a label** when all it does is fold the fragments into `CHANGELOG.md` and delete them. A `chore/release-vX.Y.Z` branch that changes anything else still needs its own fragment.
