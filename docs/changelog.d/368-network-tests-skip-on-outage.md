---
type: Fixed
pr: 368
---
**Ten network-tagged test suites no longer fail when a service is merely having
a bad day.** They checked a socket was open and then treated any answer as a
verdict on astrogo, so a Horizons 503 saying *temporary overload/maintenance*
failed the build. `ephemeris/kepler`'s Horizons fetch also now reports a non-200
as a status rather than as an unparseable body, which is what made its outages
invisible to the classifier.
