// Package iers provides Earth Orientation Parameter (EOP) models — DUT1
// (UT1-UTC), polar motion (XP/YP), and excess Length of Day (LOD).
//
// This package is an unexported implementation detail of [github.com/TuSKan/astrogo/epoch] —
// it exists at epoch/internal/iers precisely so nothing outside epoch/ can
// import it directly (Go's internal-package visibility rule enforces this
// at compile time, not just by convention). Application code, including
// coord and every other astrogo package, gets EOP data exclusively through
// epoch's public re-exports: [github.com/TuSKan/astrogo/epoch.EOP],
// [github.com/TuSKan/astrogo/epoch.RegisterModel], [github.com/TuSKan/astrogo/epoch.GetModel],
// [github.com/TuSKan/astrogo/epoch.EOPSource], [github.com/TuSKan/astrogo/epoch.ResetEOP],
// [github.com/TuSKan/astrogo/epoch.Coverage], [github.com/TuSKan/astrogo/epoch.SetRetryCooldown], and the
// [github.com/TuSKan/astrogo/epoch.Time.EOP] method.
//
// # Data source
//
// The global [Model] defaults to [ZeroModel] (zero EOP for every epoch).
// It can be populated directly via [RegisterModel], but the common case is
// [EnsureLoaded]'s automatic lazy load, triggered the first time a query
// (via [github.com/TuSKan/astrogo/epoch.Time.EOP], [github.com/TuSKan/astrogo/epoch.Time.UTC] or
// [github.com/TuSKan/astrogo/epoch.Time.UT1]) doesn't find
// coverage for the requested epoch:
//
//  1. Read and parse whatever finals2000A file already exists at the
//     standard cache path — no network access, no download policy. This is
//     how a pre-seeded file (hand-copied for an offline/air-gapped
//     deployment) is found, exactly like every other astrogo data source.
//  2. If that doesn't yield coverage, and astrogo/remote's download policy
//     allows it: download finals2000A.all over the network (through
//     astrogo/remote's cached fetch path) and register it.
//  3. Otherwise, the query degrades to the zero-EOP fallback described
//     below — EnsureLoaded never blocks indefinitely or errors loudly.
//
// Steps 1 and 2 go through astrogo/remote ([GetLoader]), so offline mode, the
// download policy, an endpoint override and the cache location apply to the
// bulletin as to every other file.
//
// [RegisterModel] is authoritative: once called, none of the above runs —
// the lazy loader will never replace what a caller explicitly registered,
// including RegisterModel(ZeroModel{}) for deliberate, deterministic zero
// EOP. Call [Reset] (a plain restore-to-pristine-default, not itself
// authoritative) to undo an explicit registration and let the lazy loader
// run again. [EOPSource] reports which of the above actually populated the
// current model.
//
// Importing this package (transitively, through time) never does network
// I/O, disk I/O, or registers anything on its own — the lazy load only
// happens in response to an actual EOP query, and its network step asks
// the same download policy every other astrogo download asks.
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
// failure). If that attempt doesn't help, epoch.Time degrades gracefully
// rather than propagating the failure everywhere: UT1→UTC conversion and
// the [github.com/TuSKan/astrogo/epoch.Time.EOP] accessor (used by coord.NewContext) silently
// substitute DUT1=XP=YP=0 and log a warning exactly once per process;
// epoch.Time.UT1 (UTC→UT1) is the one exception that returns the error
// directly, since it has no reasonable zero-value fallback direction. Use
// [Coverage] to check the currently-registered model's actual range rather
// than waiting for the one-time degradation warning.
package iers
