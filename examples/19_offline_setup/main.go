// Example: offline / air-gapped setup.
//
// astrogo never downloads a file without explicit consent (see README
// "Data downloads & offline usage"). This example demonstrates the three
// pieces of that story:
//
//  1. remote.SetOffline(true) — a global kill switch: every network call,
//     API or download, fails immediately with remote.ErrOffline.
//  2. eph.NewProvider against a pre-seeded remote.DataDir() — remote is the
//     only thing that ever resolves or opens these files, so a kernel
//     placed at its expected path is found with zero network access, no
//     separate local-only constructor required.
//  3. time.LoadFS — load Earth-orientation data from a local file (via
//     os.DirFS) instead of the network.
//
// Run: go run ./examples/19_offline_setup/
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	eph "github.com/TuSKan/astrogo/ephemeris"
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

	// ── 2. eph.NewProvider against a pre-seeded cache ───────────────────
	// This is the production path: pre-seed the kernel files yourself
	// (copy them into a deployment image, or let a build step run with
	// EnableDownloads once) at remote.DataDir()'s expected layout, then
	// call NewProvider exactly as usual. remote checks the filesystem
	// before ever considering a download, so this hits zero network as
	// long as the files are already there — no separate local-only
	// constructor to bypass remote with.
	fmt.Println("\n[2] eph.NewProvider against a pre-seeded cache, zero network")

	jplDir := remote.DataDir().Join("jpl").LocalPath()
	lskPath := filepath.Join(jplDir, "lsk", "naif0012.tls")
	spkPath := filepath.Join(jplDir, "planets", "de440s.bsp")

	p, err := eph.NewProvider(context.Background(), eph.Planets, "de440s")
	if err != nil {
		fmt.Printf("    no local kernels found at %s (%v)\n", jplDir, err)
		fmt.Println("    run example 09 or 11 first (with downloads enabled) to populate the")
		fmt.Println("    cache, or copy pre-built de440s.bsp/naif0012.tls files there yourself.")
	} else {
		defer p.Close() //nolint:errcheck // best-effort cleanup in example code

		state, err := p.State(eph.Mars, time.NowUTC())
		if err != nil {
			fmt.Printf("    State: %v\n", err)
		} else {
			fmt.Printf("    Mars geocentric distance: %.4f AU (from %s, %s)\n",
				state.Pos.Norm(), lskPath, spkPath)
		}
	}

	// ── 3. time.LoadFS — local EOP data ──────────────────────────────────
	fmt.Println("\n[3] time.LoadFS — local Earth-orientation data")

	iersDir := remote.DataDir().Join("iers").LocalPath()
	iersName := "finals2000A.data"
	iersPath := filepath.Join(iersDir, iersName)

	if err := time.LoadFS(os.DirFS(iersDir), iersName); err != nil {
		fmt.Printf("    no local EOP cache at %s — call time.Fetch once (with\n", iersPath)
		fmt.Println("    network access) to populate it, or ship a finals2000A file with your deployment.")
	} else {
		lo, hi, _ := time.Coverage()
		fmt.Printf("    loaded EOP data: MJD %.0f–%.0f\n", lo, hi)
	}

	// time.Fetch (not called here) is the network-backed equivalent — see
	// README "Data downloads & offline usage" for the full picture,
	// including remote.EnableDownloads and remote.SetDataDir for
	// redirecting all of this to a different location entirely.
}
