---
type: Fixed
pr: 186
---
**`votable.Read` returned rows for a document declaring no columns**, so a response that
was not a VOTable at all became an empty result set with a nil error — indistinguishable
from a query that matched nothing. It now returns the new `ErrNoFields` (#139).
