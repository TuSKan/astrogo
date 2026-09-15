---
type: Fixed
pr: 301
---
**An archive's error page no longer reads as a corrupt result set.** When a TAP
service answers a query with HTML and a 200 — a maintenance notice, a load
shedder, a login wall — the VOTable reader now reports that rather than
surfacing whatever the page eventually fails to parse on. ESA's Gaia archive did
this during a tagged run and the suite failed on `XML syntax error on line 161:
unexpected end element </div>`, which sends the reader looking for a parser bug
instead of at somebody else's outage; `TestArchivesAgree` now skips, as it
already did when a front end accepts a connection and stops answering.
