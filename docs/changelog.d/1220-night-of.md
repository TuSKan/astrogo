---
type: Added
pr: 1220
---
**`plan.NightOf`** returns the `plan.Night`, dusk to dawn for a twilight kind, that an instant is in, or the one after it when the instant is in daylight. Where the Sun does not cross the threshold for a day it is `plan.ErrNoTwilight`; a kind it does not know is the new `plan.ErrUnknownTwilightKind`.
