---
type: Fixed
pr: 789
---
**`catalog/gaia`'s `ConeSearch` says when the archive cut its answer, and does not cache it.** ESA's archive caps a synchronous answer at 50,000 rows, and as CSV a cut answer looked complete, so a larger `Limit` came back short and was cached. ESA is now asked for a VOTable, whose status reports the cut, and the sources it did send are followed by an error wrapping `resolve.ErrTruncated`.
