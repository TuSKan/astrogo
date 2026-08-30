---
type: Changed
pr: 75
---
**`astrogo/time` is now the module's only import of the standard library's
`time`.** A guard test enforces it, and `time` gained `GoTime`, `GoDate`,
`LocationUTC`, `Now`, the sub-second units and the timer constructors so no
call site has to reach past it.
