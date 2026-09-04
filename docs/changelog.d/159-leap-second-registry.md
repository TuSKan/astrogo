---
type: Added
pr: 159
---
**A ΔAT table can now be registered, so a new leap second no longer needs a
release.** `time.RegisterLeapSeconds` installs a published table process-wide;
`LeapSecondSource`/`ResetLeapSeconds` mirror the EOP registry. Registration is
superset-only — a table that contradicts the built-in record below its last
step is refused, which is what makes registering late safe. `jpl.NewProvider`
registers its kernel's `DELTA_AT` block, so `time`'s scale conversions and the
ET the SPK is evaluated at follow one source (#143).
