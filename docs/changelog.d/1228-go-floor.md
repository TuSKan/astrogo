---
type: Changed
pr: 1228
---
**Documentation:** the README states that astrogo needs Go 1.27 or later, and why: `remote` decodes JSON with `encoding/json/v2`, a standard package from Go 1.27, and no dependency needs more than Go 1.26.
