// Package iers provides Earth Orientation Parameter (EOP) models — DUT1
// (UT1-UTC), polar motion (XP/YP), and excess Length of Day (LOD).
//
// This package is an unexported implementation detail of [astrogo/time] —
// it exists at time/internal/iers precisely so nothing outside time/ can
// import it directly (Go's internal-package visibility rule enforces this
// at compile time, not just by convention). Application code, including
// coord and every other astrogo package, gets EOP data exclusively through
// time's public re-exports: [time.EOP], [time.Fetch], [time.FetchIfStale],
// [time.LoadFS], [time.RegisterModel], [time.GetModel], [time.Coverage],
// [time.SetRetryCooldown], and the [time.Time.EOP] method.
//
// # Data source
//
// The global [Model] defaults to [ZeroModel] (zero EOP for every epoch).
// Populate it explicitly via exactly one of:
//
//   - [Fetch] / [FetchIfStale] — download finals2000A.all over the network
//     (through astrogo/remote's consent-gated, cached fetch path) and
//     register it
//   - [LoadFS] — parse a finals2000A file reached through any io/fs.FS (a
//     local directory via os.DirFS, a downstream application's own
//     embed.FS, or any other stdlib-compatible filesystem) and register it
//
// There is no build-time data and no implicit loading — importing this
// package (transitively, through time) never does network I/O, disk I/O,
// or registers anything on its own.
//
// A registered [*Table] has a hard, finite coverage window — it only
// contains rows up to whatever finals2000A.all's tail extended to when it
// was fetched/loaded, and the trailing weeks of even that range are
// typically "predicted" placeholders (blank measured DUT1/XP/YP fields,
// which the parser correctly represents as zero).
//
// # What happens when the data runs out
//
// Once real time advances past the registered model's coverage, every call
// to [Model.EOP] for a later epoch returns [ErrOutOfRange]. time.Time
// degrades gracefully rather than propagating this everywhere: UT1→UTC
// conversion and the [time.Time.EOP] accessor (used by coord.NewContext)
// silently substitute DUT1=XP=YP=0 and log a warning exactly once per
// process; time.Time.UT1 (UTC→UT1) is the one exception that returns the
// error directly, since it has no reasonable zero-value fallback direction.
// Use [Coverage] to check the currently-registered model's actual range and
// call [FetchIfStale] proactively, rather than waiting for the one-time
// degradation warning.
package iers
