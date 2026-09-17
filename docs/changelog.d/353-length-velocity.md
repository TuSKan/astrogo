---
type: Added
pr: 353
---
`unit.Length` and `unit.Velocity` are named `float64` types carrying metres and
metres per second, with constructors and accessors for every unit astrogo
speaks — `unit.AU`, `Km`, `Pc`, `Meters`, `KmPerSec`, `AUPerDay` and the
readers that match. They are the pattern `angle.Angle` already uses, extended
to the two dimensions the API was passing as bare `float64`. Measured, a named
`float64` is indistinguishable from the `float64` it replaces (1.88 ns against
1.88 ns scalar; 1887 ns against 1888 ns over a 1000-element batch; 8 bytes
either way), while `unit.Quantity` is 56 bytes and 14.3× slower over that batch
— neither allocates, so the difference is width and cache pressure rather than
the heap. New allocation contracts in `unit` hold that claim. The constructors
read their scale factors from this package's own `Unit` table rather than from
constants of their own, so a `Length` and a `Quantity` cannot come to disagree
about how long an astronomical unit is; `Length.Quantity` and `unit.LengthFrom`
bridge the two when a value has to compose dimensionally.
