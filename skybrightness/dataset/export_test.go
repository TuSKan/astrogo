package dataset

// NewSky exposes the assembly half of [Open] — everything after the fetch —
// so a test can build a Sky from inputs it already holds.
//
// Test-only, so the package's surface still offers exactly one constructor
// and that constructor still gathers its own data. What this buys is that the
// wiring questions worth asking about a Sky — which observer reaches the
// scene, which transfer is applied, which magnitude system is reported — get
// answered on every commit instead of only when a 145 MB download succeeds.
var NewSky = newSky
