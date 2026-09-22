---
type: Changed — BREAKING
pr: 364
---
**Five exported names lose their British spelling**, which is the whole
exported surface that had one:

| package | before | after |
| --- | --- | --- |
| `ephemeris`, `ephemeris/core` | `CenterGeocentre` | `CenterGeocenter` |
| `ephemeris`, `ephemeris/core` | `CenterBarycentre` | `CenterBarycenter` |
| `ephemeris`, `ephemeris/core` | `CenterHeliocentre` | `CenterHeliocenter` |
| `magnitude` | `JohnsonCousinsColourTerm` | `JohnsonCousinsColorTerm` |
| `skybrightness` | `ZodiacalColourCorrection` | `ZodiacalColorCorrection` |

The three `Center` constants spelled it both ways inside one identifier — an
American prefix on a British suffix. astrogo's convention is American and is
already stated in code: `unit/units.go` declares `Name: "meter"`.

`Center.String()` follows, so it now returns `"geocenter"`, `"barycenter"` and
`"heliocenter"`. A caller matching on those strings needs updating; nothing in
this repository parses them.
