---
type: Fixed
pr: 215
---
**`catalog/fink` reported an asteroid it had never heard of as an outage.** FINK
answers an unknown identifier with a `RemoteException`, which read as a failure
and was joined into the returned error, so `Resolve` never produced
`ErrNotFound` — #102's inversion. The bulk SSOFT table loading and not holding
the object now settles it, and the exception's own message is carried instead of
discarded (#122).
