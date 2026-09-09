---
type: Fixed
pr: 251
---
`ephemeris/jpl/spk`'s Horizons status sentinels now wrap the HTTP error rather
than replacing it, so a 503 is recognisable as upstream downtime by anything
matching `HTTPStatus() int` — previously a service outage was indistinguishable
from a bad request (#251).
