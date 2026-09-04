---
type: Fixed
pr: 148
---
**The kernel-driven ET no longer swallows historical ΔT.** The kernel-driven conversion now
delegates to `time` for epochs before the `DELTA_AT` table's first entry
(1972-01-01), where leap seconds do not apply and the offset is the Espenak &
Meeus (2006) ΔT instead — worth 175 minutes at year 1, which surfaced as ~180
minute errors across the AstroPixels year-0001 lunar phases.
