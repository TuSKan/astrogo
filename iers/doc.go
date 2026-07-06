// Package iers provides Earth Orientation Parameter (EOP) models — DUT1
// (UT1-UTC), polar motion (XP/YP), and excess Length of Day (LOD) — used
// throughout astrogo wherever a SOFA routine needs UT1 or polar motion
// (chiefly coord.NewContext and time.Time's UT1 conversion).
//
// # Data source and freshness
//
// This package's go:generate directive (iers.go) downloads the IERS
// "finals2000A.all" bulletin into iers/data/ at build time, and an init()
// parses it into a [*Table] and registers it as the global [Model] via
// [RegisterModel]. That table has a hard, finite coverage window — it only
// contains rows up to whatever finals2000A.all's tail extended to on the day
// `go generate` last ran, and the trailing weeks of even that range are
// typically "predicted" placeholders (blank measured DUT1/XP/YP fields).
//
// Because the embedded data file is not committed to the repository (it is
// regenerated, not stored), the actual coverage window in any given build
// depends entirely on how recently `go generate ./...` ran before that build
// — check [GetModel]'s [Model.EOP] against [Coverage] at startup in any
// long-running service, rather than assuming freshness.
//
// # What happens when the data runs out
//
// Once real time advances past the registered model's coverage, every call
// to [Model.EOP] for a later epoch returns [ErrOutOfRange]. The two call
// sites that consume it in the rest of astrogo (coord.NewContext and
// time.Time's internal UT1 lookup) do NOT propagate that error to their own
// callers — coord.NewContext has no error return at all, and time's fallback
// silently substitutes DUT1=XP=YP=0. Both log a warning exactly once per
// process (via sync.Once), not once per call, so a long-running service only
// gets a single log line for the entire remainder of its life, and a
// short-lived process that starts already past the coverage window gets
// exactly the same single warning as one that only crosses the boundary
// mid-run. The resulting positional error from zeroed DUT1/polar motion is on
// the order of ~1 arcsecond — real, but easy to miss if the log isn't
// monitored.
//
// [FetchIfStale] exists to explicitly refresh the registered model at
// runtime, but nothing in astrogo calls it automatically — it is only useful
// if application code calls it itself, proactively, before the coverage
// window is exhausted. Use [Coverage] to check the currently-registered
// model's actual range and decide when to call [FetchIfStale] (or simply
// regenerate the embedded data at deploy time) rather than waiting for the
// one-time degradation warning.
package iers
