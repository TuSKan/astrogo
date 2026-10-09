---
type: Fixed
pr: 686
---
**The USNO celestial-navigation tests compared refracted with airless altitudes and counted the Sun's aberration twice**, inside tolerances of 0.1° to 1.5°, so the "0.002°" quoted for Sirius was its refraction at 83°. `TestUSNO_CelNav` is now an offline fixture asking astrogo USNO's own question: airless and geocentric, at UT1. The Sun and Sirius agree within 0.67″ at five places, held to 2″. The library was right; only the test was not.
