---
type: Fixed
pr: 167
---
**`Event.Altitude` was geometric at rise/set and refracted at transit**, with
nothing in the type saying so, and `Event.GeometricAltitude` was assigned the
identical value at both construction sites — so one field told a caller nothing
the other did, and comparing a rise altitude against a transit altitude
compared two different quantities. `Altitude` is now the refracted altitude at
every event kind, matching `IsObservable` and `GetDetails`; `GeometricAltitude`
is the unrefracted one. `Value` stays geometric at rise/set, so event times are
unchanged (#156).
