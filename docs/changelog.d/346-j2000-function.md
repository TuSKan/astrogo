---
type: Changed — BREAKING
pr: 346
---
**`time.J2000` is now a function, `time.J2000()`.** It was the last symbol
astrogo itself exported as a mutable package-level variable, so any package
anywhere in the import graph could reassign the standard epoch and change every
epoch calculation in the process — a supply-chain footgun in the package that
underpins the rest of the library. Go cannot declare a struct value immutable,
so a function returning a copy of an unexported one is the only construction
that removes it; the value is computed once and the call costs a struct copy.
Callers write `time.J2000()` wherever they wrote `time.J2000`, and the compiler
finds every site. `time.LocationUTC` remains a var and is now the only one that
is not a sentinel error: the standard library declares `var UTC *Location`, so
wrapping it would hand back the same reassignable pointer and remove nothing.
README's claim is corrected to say that rather than to imply nothing is
reassignable at all, and `internal/docsguard`'s inventory of remaining vars
records why each survives. Closes [#113].
