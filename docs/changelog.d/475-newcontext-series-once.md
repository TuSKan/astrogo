---
type: Changed
pr: 475
---
**`coord.NewContext` is 34% faster** (138 → 91 µs), with bit-identical
results: it reuses the precession-nutation matrix `Apco13` already built
instead of evaluating the series again, and a work contract now holds it to
one evaluation (#473).
