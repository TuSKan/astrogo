---
type: Removed
pr: 539
---
**`ephemeris`'s `Body`, `Kind` and body table are gone** (since removed: `core.Body`, `core.Kind` and its constants, `core.SunBody` … `core.NeptuneBody`, `core.Bodies`, and their `ephemeris` re-exports).
Nothing took or returned a `Body`; use `core.ID`, whose `String` gives the name.
Eleven of them were reassignable globals, so one importer could redefine the Sun for the whole process.
