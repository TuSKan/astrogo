---
type: Added
pr: 196
---
**`satellite.Satellite.Verified`** reports whether an element set sits inside the regime
astrogo's SGP4 verification covers, and says why when it does not — so a position that may
be 3440 km out no longer looks exactly like one for the ISS. It tests perigee against
SGP4's own 220 km simplified-drag branch; deep space is deliberately excluded, because the
measurement says it would raise fifteen false alarms and catch nothing new (#182).
