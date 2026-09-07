---
type: Added
pr: 215
---
Offline tests for `catalog/fink`'s SSOFT bulk-table path, taking the package
from 30.7% to 87.0% — the parquet load and the fit/status filter on it, the four
ways the download is not the table, the JSON coercions the single-object
endpoint needs, and absence against failure in `Search` and `ResolveObject`
(#122).
