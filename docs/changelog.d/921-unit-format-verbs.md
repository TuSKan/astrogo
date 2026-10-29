---
type: Fixed
pr: 921
---
**`coord.ObserversLocation.String` names its height's unit, and a float verb can no longer print a raw unit or angle value.** The height printed in meters with no unit; it now ends in " m". A new docsguard test type-checks every package and fails on `%f`, `%g` or `%e` applied to a `unit` or `angle` value that has a float view of itself, which is how six test messages came to print m/s as km/s, radians as degrees, seconds as minutes and meters as parsecs.
