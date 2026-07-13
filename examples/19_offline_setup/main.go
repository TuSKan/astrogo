// Example: offline / air-gapped setup.
//
// astrogo never downloads a file without explicit consent (see README
// "Data downloads & offline usage"). This example demonstrates the three
// pieces of that story:
//
//  1. remote.SetOffline(true) — a global kill switch: every network call,
//     API or download, fails immediately with remote.ErrOffline.
//  2. jpl.Open — construct a JPL ephemeris provider purely from local
//     kernel files, with zero network access at all (no registry gate to
//     even trip).
//  3. iers.LoadFile — load Earth-orientation data from a local file
//     instead of the network or the build-time embedded snapshot.
//
// Run: go run ./examples/19_offline_setup/
package main

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/iers"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

func main() {
	fmt.Println("=== astrogo offline / air-gapped setup ===")

	// ── 1. The kill switch ──────────────────────────────────────────────
	// With offline mode on, no endpoint — API or download — may be
	// contacted, regardless of any EnableDownloads/Enable calls made
	// elsewhere in the process. remote.URL is the gate every call site
	// (Client.Get, Download, jpl's CacheDownload) goes through — calling
	// it directly demonstrates the kill switch without depending on
	// whether a kernel happens to already be cached on this machine (a
	// pre-seeded eph.NewProvider call would succeed straight from disk
	// even while offline, per point 2 below — that's the whole point of
	// pre-seeding, not a bug).
	remote.SetOffline(true)
	fmt.Println("\n[1] remote.SetOffline(true) is active.")

	_, err := remote.URL(remote.NAIFSPK)
	if errors.Is(err, remote.ErrOffline) {
		fmt.Println("    remote.URL correctly refused to resolve an endpoint:")
		fmt.Printf("      %v\n", err)
	} else {
		fmt.Printf("    unexpected: %v\n", err)
	}

	remote.SetOffline(false) // restore for the rest of this example

	// ── 2. jpl.Open — pure local construction ───────────────────────────
	// This is the production path: pre-seed the kernel files yourself
	// (copy them into a deployment image, or let a build step run with
	// EnableDownloads once and reuse remote.DataDir() afterward), then
	// Open them directly. No registry gate is even consulted.
	fmt.Println("\n[2] jpl.Open — pure local construction, zero network")

	jplDir := remote.DataDir().Join("jpl").LocalPath()
	lskPath := filepath.Join(jplDir, "lsk", "naif0012.tls")
	spkPath := filepath.Join(jplDir, "planets", "de440s.bsp")

	p, err := jpl.Open(lskPath, spkPath)
	if err != nil {
		fmt.Printf("    no local kernels found at %s — run example 09 or 11 first\n", jplDir)
		fmt.Println("    (with downloads enabled) to populate the cache, then re-run this example.")
	} else {
		defer p.Close() //nolint:errcheck // best-effort cleanup in example code

		state, err := p.State(core.Mars, time.NowUTC())
		if err != nil {
			fmt.Printf("    State: %v\n", err)
		} else {
			fmt.Printf("    Mars geocentric distance: %.4f AU (from %s, %s)\n",
				state.Pos.Norm(), lskPath, spkPath)
		}

		for i, k := range p.LoadedKernels() {
			fmt.Printf("    kernel[%d]: %s (%d segments)\n", i, k.Path, k.Segments)
		}
	}

	// ── 3. iers.LoadFile — local EOP data ───────────────────────────────
	fmt.Println("\n[3] iers.LoadFile — local Earth-orientation data")

	iersPath := remote.DataDir().Join("iers", "finals2000A.data").LocalPath()
	if err := iers.LoadFile(iersPath); err != nil {
		fmt.Printf("    no local EOP cache at %s — call iers.FetchNow once (with\n", iersPath)
		fmt.Println("    network access) to populate it, or ship a finals2000A file with your deployment.")
	} else {
		lo, hi, _ := iers.Coverage()
		fmt.Printf("    loaded EOP data: MJD %.0f–%.0f\n", lo, hi)
	}

	// FetchNow (not called here) is the network-backed equivalent — see
	// README "Data downloads & offline usage" for the full picture,
	// including remote.EnableDownloads and remote.SetDataDir for
	// redirecting all of this to a different location entirely.
}
