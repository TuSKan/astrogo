---
type: Added
pr: 313
---
`spk.ErrHorizonsInternalFault` and `spk.TransientHorizonsFault` separate a JPL
Horizons outage from a Horizons refusal. Both arrive as HTTP 200 with a
well-formed body and mean opposite things: one resolves itself, the other never
will. Retry logic and astrogo's own live-network tests now branch on the
difference instead of treating every refusal alike. [#312]
