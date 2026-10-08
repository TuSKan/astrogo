---
type: Fixed
pr: 631
---
**`plan.VisibleTonight` charged ozone the molecular airmass**, three times the airmass of its thin shell 20 km up at the horizon, so a low target was dimmed 0.63 mag too much at 0° and 0.34 at 1° on the default air. New `atmosphere.Atmosphere.ExtinctionToward` dims each term of the air through its own airmass, and VisibleTonight uses it at each target's peak.
