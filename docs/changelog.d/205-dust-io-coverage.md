---
type: Added
pr: 205
---
Offline tests for `skybrightness/dataset/dust`'s I/O paths, taking it from 37.5%
to 96.5% — the IRSA fetch and its cache (a second session asks nothing, a cell is
asked once, a run cut off keeps what it paid for, a corrupt line costs one
sightline) and every reason `SFD.Open` refuses a hemisphere, against synthetic
SFD-shaped FITS built in the test (#122).
