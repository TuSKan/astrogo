// Package iers provides Earth Orientation Parameter (EOP) models — DUT1
// (UT1-UTC), polar motion (XP/YP), and excess Length of Day (LOD).
//
// This package is an unexported implementation detail of [astrogo/time] —
// it exists at time/internal/iers precisely so nothing outside time/ can
// import it directly (Go's internal-package visibility rule enforces this
// at compile time, not just by convention). Application code, including
// coord and every other astrogo package, gets EOP data exclusively through
// time's public re-exports: [time.EOP], [time.RegisterModel],
// [time.GetModel], [time.Coverage], [time.SetRetryCooldown], and the
// [time.Time.EOP] method.
//
// # Data source
//
// The global [Model] defaults to [ZeroModel] (zero EOP for every epoch).
// It can be populated directly via [RegisterModel], but the common case is
// [EnsureLoaded]'s automatic lazy load, triggered the first time a query
// (via [time.Time.EOP]/[time.Time.UTC]/[time.Time.UT1]) doesn't find
// coverage for the requested epoch:
//
//  1. Read and parse whatever finals2000A file already exists at the
//     standard cache path — no network access, no consent check. This is
//     how a pre-seeded file (hand-copied for an offline/air-gapped
//     deployment) is found, exactly like every other astrogo data source.
//  2. If that doesn't yield coverage, and astrogo/remote.EnableDownloads
//     was called for remote.IERSFinals2000A: download finals2000A.all over
//     the network (through astrogo/remote's consent-gated, cached fetch
//     path) and register it.
//  3. Otherwise, the query degrades to the zero-EOP fallback described
//     below — EnsureLoaded never blocks indefinitely or errors loudly.
//
// Importing this package (transitively, through time) never does network
// I/O, disk I/O, or registers anything on its own — the lazy load only
// happens in response to an actual EOP query, and its network step is
// still gated by the same download consent every other astrogo data
// source requires.
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
// to [Model.EOP] for a later epoch returns [ErrOutOfRange], triggering
// another lazy-load attempt (throttled by [SetRetryCooldown] after a
// failure). If that attempt doesn't help, time.Time degrades gracefully
// rather than propagating the failure everywhere: UT1→UTC conversion and
// the [time.Time.EOP] accessor (used by coord.NewContext) silently
// substitute DUT1=XP=YP=0 and log a warning exactly once per process;
// time.Time.UT1 (UTC→UT1) is the one exception that returns the error
// directly, since it has no reasonable zero-value fallback direction. Use
// [Coverage] to check the currently-registered model's actual range rather
// than waiting for the one-time degradation warning.
package iers
