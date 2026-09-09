---
type: Changed
pr: 233
---
Every JSON API response now decodes through `encoding/json/v2` — measured on an
SBDB-shaped body at 24.5 µs against 37.4 µs, 12.1 KB against 28.8 KB, and 9
allocations against 20. A response repeating an object name is now an error
rather than silently taking the last occurrence; case-insensitive field
matching and tolerance of invalid UTF-8 are deliberately kept, because exact
matching would leave a renamed coordinate at RA 0, Dec 0 (#126-adjacent, see
`plan.ErrNoCoordinates`).
