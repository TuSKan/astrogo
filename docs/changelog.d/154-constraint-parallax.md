---
type: Fixed
pr: 154
---
**The scheduler treated the Moon as a star at infinity.** Every constraint,
score and visibility check called `ICRSToAltAz` on a geocentric position,
discarding the observer's offset from the geocentre — up to **0.95°** for the
Moon, so an `Altitude{Threshold: 0}` constraint reported it up about four
minutes before `MoonEvents` said it rose. Crescent visibility was affected
worst, its own comment claiming "topocentric" while the code was not.
