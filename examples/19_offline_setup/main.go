// Example: offline / air-gapped setup.
//
// astrogo downloads what a call needs when it first needs it (see README
// "Data downloads & offline usage"). A program that must not touch the
// network turns it off once, and runs on files placed in the cache
// beforehand. This example demonstrates the three pieces of that story:
//
//  1. remote.SetOffline(true) — every network call, API or download, fails
//     immediately with remote.ErrOffline. It stays on for the whole run.
//  2. eph.NewProvider against a pre-seeded remote.DataDir() — remote is the
//     only thing that ever resolves or opens these files, so a kernel
//     placed at its expected path is found with zero network access, no
//     separate local-only constructor required.
//  3. epoch.LoadEOP against a pre-seeded finals2000A.data — Earth-orientation
//     data follows the exact same rule as the kernel above.
//
// Run: go run ./examples/19_offline_setup/
package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/epoch"
	"github.com/TuSKan/astrogo/examples/internal/show"
	"github.com/TuSKan/astrogo/remote"
)

// cacheLocation names where an endpoint's files are expected, for display
// only. Nothing here is assumed to be on local disk: the data directory is
// a bucket URL and the cache is a key prefix within it, so this prints
// exactly the two pieces a pre-seeding operator needs, whether that bucket
// is a directory, an S3 bucket, or anything else.
func cacheLocation(ctx context.Context, id remote.EndpointID, name string) string {
	_, prefix, err := remote.CacheDir(ctx, id)
	if err != nil {
		return name
	}

	return remote.DataDirURL() + " [" + prefix + name + "]"
}

func main() {
	show.Title("Offline and air-gapped setup",
		"No network at all; pre-seeded files just work")

	// ── 1. The kill switch ──────────────────────────────────────────────
	// With offline mode on, no endpoint — API or download — may be
	// contacted. remote.URL is the gate every call site (Client.Get,
	// GetFile, jpl's kernel generation) goes through — calling it directly
	// demonstrates the switch without depending on whether a kernel
	// happens to already be cached on this machine (a pre-seeded
	// eph.NewProvider call succeeds straight from disk while offline, per
	// point 2 below — that's the whole point of pre-seeding, not a bug).
	// It stays on for the rest of this example.
	remote.SetOffline(true)

	show.Section("1. The kill switch: remote.SetOffline(true)")

	_, err := remote.URL(remote.NAIFSPK)
	if errors.Is(err, remote.ErrOffline) {
		show.Fields(show.KV("remote.URL", "refused, as it must: %v", err))
	} else {
		show.Fields(show.KV("remote.URL", "unexpected: %v", err))
	}

	// ── 2. eph.NewProvider against a pre-seeded cache ───────────────────
	// This is the production path: pre-seed the kernel files yourself
	// (copy them into a deployment image, or run once with network access)
	// at remote.DataDir()'s expected layout, then call NewProvider exactly
	// as usual. remote checks the cache before the network, and offline it
	// goes no further, so this hits zero network as long as the files are
	// already there — no separate local-only constructor to bypass remote
	// with.
	show.Section("2. A pre-seeded kernel, zero network")

	ctx := context.Background()
	lskLoc := cacheLocation(ctx, remote.NAIFLSK, "naif0012.tls")
	spkLoc := cacheLocation(ctx, remote.NAIFSPK, "planets/de440s.bsp")

	p, err := eph.NewProvider(ctx, eph.Planets, "de440s")
	if err != nil {
		show.Note(fmt.Sprintf("No cached kernels at %s (%v).", remote.DataDirURL(), err),
			"Run example 09 or 11 first, with network access, to populate the",
			"cache, or copy pre-built de440s.bsp and naif0012.tls there yourself.")
	} else {
		defer p.Close() //nolint:errcheck // best-effort cleanup in example code

		state, err := eph.At(p, epoch.Date(2026, 4, 6, 22, 0, 0, 0, time.UTC)).Geometric(eph.MarsBarycenter, eph.Earth)
		if err != nil {
			show.Fields(show.KV("Geometric", "%v", err))
		} else {
			show.Fields(
				show.KV("Mars, geocentric", "%.4f AU", state.Pos.Norm()),
				show.KV("Leap seconds", "%s", lskLoc),
				show.KV("Kernel", "%s", spkLoc),
			)
		}
	}

	// ── 3. epoch.LoadEOP — local Earth-orientation data, loaded explicitly ──
	show.Section("3. Earth orientation, loaded by epoch.LoadEOP")

	iersLoc := cacheLocation(ctx, remote.IERSFinals2000A, "finals2000A.data")

	// Offline, LoadEOP's network step is refused before any request and it
	// reads the cache. Unlike the lazy load a Time.EOP() triggers, it says
	// when there is nothing to read.
	if err := epoch.LoadEOP(ctx); err != nil {
		show.Note(fmt.Sprintf("No local EOP cache at %s.", iersLoc),
			"Run any program that needs it once, with network access, to",
			"populate it, or ship finals2000A.data with your deployment.",
			fmt.Sprintf("epoch.LoadEOP: %v", err))
	} else {
		lo, hi, _ := epoch.Coverage()
		show.Fields(
			show.KV("Coverage", "MJD %.0f–%.0f", lo, hi),
			show.KV("Bulletin", "%s", iersLoc),
			show.KV("Source", "%s", epoch.EOPSource()),
		)
	}

	// Earth-orientation data works exactly like the kernel above: pre-seed
	// finals2000A.data at this path, or let one run with network access
	// fetch it — see README "Data downloads & offline usage" for the full
	// picture, including remote.SetDataDir for redirecting all of this to a
	// different location entirely.
}
