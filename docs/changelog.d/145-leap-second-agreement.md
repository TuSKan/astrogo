---
type: Added
pr: 145
---
**A tripwire for astrogo's two leap-second sources.**
`TestLeapSecondSourcesAgree` compares NAIF's kernel against the table compiled
into gofa in both directions — every kernel entry, plus an independent
1972–2035 sweep that is the half able to see an entry beyond gofa's last one.
They agree today; when they stop, the pinned table is stale (#143).
