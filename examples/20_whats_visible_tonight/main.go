// Package main answers the question "what can I see in the sky tonight
// brighter than magnitude X?" — the single plan.VisibleTonight entry point
// composing catalog bright-object search (SIMBAD stars, OpenNGC deep sky,
// SBDB asteroids/comets), the Moon and naked-eye planets, rise/transit/set
// timing, atmospheric extinction, and constellation lookup into one report.
//
// This example demonstrates:
//   - plan.VisibleTonight across every object category the library covers
//   - catalog.NewProvider constructing each source by Source constant,
//     without importing catalog/simbad, catalog/openngc, or catalog/sbdb
//     directly
//   - The two-stage asteroid/comet design (catalog/sbdb.SearchBright's cheap
//     bulk prefilter, then a real per-body JPL Horizons SPK fetch inside
//     VisibleTonight for the ones that pass it)
//   - constellation.Lookup annotating every result
//   - Rise/Transit/Peak/Set timing and a compass Direction for where to
//     look, all evaluated at the real best-observed instant
//   - magLimit governing every category uniformly — no special-cased
//     inclusion/exclusion by object type
//
// NOT enabled here: plan.WithPlanetaryMoons(), which adds the 21 major
// natural satellites of Mars/Jupiter/Saturn/Uranus/Neptune/Pluto (Io,
// Titan, Triton, ...) as candidates. It's opt-in rather than default
// because their SPK kernels are far larger than everything else this
// example downloads — from ~64 MB (Mars) up to ~1.1 GB (Jupiter's
// Galilean moons), ~2.4 GB total for every kernel. To try it: pass
// plan.WithPlanetaryMoons() to the VisibleTonight call below, raise this
// example's remote.EnableDownloads(remote.NAIFSPK, ...) cap accordingly,
// and loosen magLimit — every one of these moons is fainter than mag 4 in
// real apparent brightness (Titan/Triton, the brightest, are ~+8.4/+13.5),
// so magLimit=2 (this example's default) will never surface any of them
// even with the kernels present.
//
// Run: go run ./examples/20_whats_visible_tonight/
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/catalog/resolve"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

// magLimit is the brightness bound for this run — everything fainter than
// this, in every category, is excluded. Try 5 to see how asteroids/comets
// start to legitimately appear alongside the mag-2-and-brighter naked-eye
// staples.
const magLimit = 2.0

// brightSource builds one resolve.BrightObjectSearcher via catalog.NewProvider
// — never by importing the provider's subpackage directly — and fails
// loudly if that source doesn't actually implement the capability (a
// programmer error in this example, not a runtime condition to recover
// from).
func brightSource(src catalog.Source) resolve.BrightObjectSearcher {
	p, err := catalog.NewProvider(src)
	if err != nil {
		log.Fatalf("catalog.NewProvider: %v", err)
	}

	bs, ok := p.(resolve.BrightObjectSearcher)
	if !ok {
		log.Fatalf("provider %q does not implement resolve.BrightObjectSearcher", p.Name())
	}

	return bs
}

func main() {
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("  What's Visible Tonight")
	fmt.Printf("  Every known object brighter than mag %.1f\n", magLimit)
	fmt.Println("═══════════════════════════════════════════════════════════════")

	ctx := context.Background()

	// ── Consent-gated downloads ────────────────────────────────────────
	// See README "Data downloads & offline usage" — nothing downloads
	// without this. IERS EOP (~4 MB) improves rise/set precision; OpenNGC's
	// two source CSVs (~7 MB) back the deep-sky category; de440s.bsp
	// (~32 MB) backs the Moon/planets and, via VisibleTonight's Stage 2,
	// any asteroid/comet candidate SIMBAD/OpenNGC/SBDB's bulk prefilter
	// surfaces; naif0012.tls (~6 KB) provides leap seconds for all of them.
	remote.EnableDownloads(remote.IERSFinals2000A, 0)
	remote.EnableDownloads(remote.OpenNGC, 0)
	remote.EnableDownloads(remote.NAIFSPK, 200<<20)
	remote.EnableDownloads(remote.NAIFLSK, 0)
	remote.EnableDownloads(remote.JPLHorizons, 0)

	planetProvider, err := eph.NewProvider(ctx, eph.Planets, "de440s")
	if err != nil {
		log.Fatalf("failed to load JPL DE440s: %v", err)
	}
	defer func() {
		if err := planetProvider.Close(); err != nil {
			log.Printf("failed to close planet provider: %v", err)
		}
	}()

	brtz, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		log.Fatalf("failed to load timezone: %v", err)
	}

	site, err := plan.NewSiteEarthLocation("Quinta Calixto", -22.528478, -46.473002, 835.05, plan.WithTimeZone(brtz))
	if err != nil {
		log.Fatalf("NewSiteEarthLocation: %v", err)
	}

	// ── Bright-object sources ───────────────────────────────────────────
	// Stars (SIMBAD), Deep Sky — nebulae/clusters/galaxies (OpenNGC), and
	// asteroids+comets (SBDB, Stage 1 of the two-stage design — see
	// catalog/sbdb.SearchBright's doc comment). Any resolve.BrightObjectSearcher
	// can be added here; VisibleTonight doesn't care which providers a
	// caller registers.
	sources := []resolve.BrightObjectSearcher{
		brightSource(catalog.SIMBAD),
		brightSource(catalog.OpenNGC),
		brightSource(catalog.SBDB),
	}

	// A fixed date keeps this example's output reproducible run to run —
	// August 1, 2026, a clear midwinter night from Quinta Calixto's southern
	// hemisphere. night should be an instant earlier in the same calendar
	// day as dusk; local midnight is the natural choice.
	night := time.Date(2026, time.August, 1, 0, 0, 0, 0, brtz)

	fmt.Printf("\nSite: %s (%.4f, %.4f)\n", site.Name(), -22.528478, -46.473002)
	fmt.Printf("Night of: %s\n", night.Format("2006-01-02"))
	fmt.Println("Fetching bright-object candidates and computing visibility — this")
	fmt.Println("does real network queries (SIMBAD/OpenNGC/SBDB) and, for any")
	fmt.Println("qualifying minor body, a real JPL Horizons ephemeris fetch...")

	results, err := plan.VisibleTonight(ctx, site, night, magLimit, sources, planetProvider)
	if err != nil {
		log.Fatalf("VisibleTonight: %v", err)
	}

	if len(results) == 0 {
		fmt.Printf("\nNothing brighter than mag %.1f is visible tonight from %s.\n", magLimit, site.Name())
		return
	}

	printVisibleTonightTable(results, brtz)

	fmt.Println("\n═══════════════════════════════════════════════════════════════")
	fmt.Println("  Computed with astrogo — plan.VisibleTonight")
	fmt.Println("═══════════════════════════════════════════════════════════════")
}

// ── Colorized table rendering ──────────────────────────────────────────────
//
// Everything below is presentation only — plan.VisibleTonight's own result
// (VisibleObject) is plain, colorless data; this example chooses how to
// display it. Colors are ANSI escape codes (no external dependency), and are
// skipped entirely when NO_COLOR is set (https://no-color.org), matching
// the convention most terminal tools honor.

const (
	ansiReset   = "\x1b[0m"
	ansiBold    = "\x1b[1m"
	ansiDim     = "\x1b[2m"
	ansiWhite   = "\x1b[97m"
	ansiYellow  = "\x1b[33m"
	ansiBYellow = "\x1b[93m"
	ansiCyan    = "\x1b[36m"
	ansiGreen   = "\x1b[32m"
	ansiMagenta = "\x1b[35m"
	ansiBlue    = "\x1b[34m"
	ansiGray    = "\x1b[90m"
)

var colorEnabled = os.Getenv("NO_COLOR") == ""

// colorize wraps s in an ANSI color/style code, a no-op when colorEnabled is
// false. Callers must colorize an already width-padded string — an escape
// code is zero-width on screen but not to len(), so padding after wrapping
// would misalign every column.
func colorize(code, s string) string {
	if !colorEnabled || code == "" {
		return s
	}

	return code + s + ansiReset
}

// kindColor groups resolve.Kind values by visual category — moving/solar
// system bodies, stars, and deep-sky objects each get a distinct color
// family, mirroring how a planetarium app color-codes its object list.
func kindColor(k resolve.Kind) string {
	switch k { //nolint:exhaustive // KindSatellite/KindOther intentionally fall to the default gray
	case resolve.KindMoon:
		return ansiBold + ansiWhite
	case resolve.KindPlanet, resolve.KindDwarfPlanet:
		return ansiCyan
	case resolve.KindStar, resolve.KindDoubleStar:
		return ansiYellow
	case resolve.KindGalaxy:
		return ansiMagenta
	case resolve.KindNebula, resolve.KindStarCluster, resolve.KindOpenCluster,
		resolve.KindGlobularCluster, resolve.KindSupernovaRemnant, resolve.KindAsterism:
		return ansiBlue
	case resolve.KindAsteroid, resolve.KindComet:
		return ansiGreen
	default:
		return ansiGray
	}
}

// magColor grades a row by how bright it actually is, brightest-catches-the-
// eye first — the same instinct as a star chart's dot-size legend, done in
// color instead since this is text.
func magColor(mag float64) string {
	switch {
	case mag < 0:
		return ansiBold + ansiBYellow
	case mag < 1:
		return ansiYellow
	case mag < 1.5:
		return ansiWhite
	default:
		return ansiGray
	}
}

// tableRow is one already-formatted (but not yet padded or colored) line of
// the table, plus the raw values still needed to pick colors and to render
// footnotes.
type tableRow struct {
	num, object, kind, mag, constellation, rise, peak, alt, dir, set string
	kindRaw                                                          resolve.Kind
	magVal                                                           float64
	note                                                             string
}

// printVisibleTonightTable renders results as a box-drawn, column-aligned,
// color-coded table. Column widths are computed from the actual data (not
// hardcoded), so it stays correctly sized whether magLimit surfaces 5
// objects or 50.
func printVisibleTonightTable(results []plan.VisibleObject, loc *time.Location) {
	clock := func(t time.Time) string {
		if t.IsZero() {
			return "—"
		}

		return t.In(loc).Format("15:04")
	}

	headers := []string{"#", "Object", "Kind", "Mag", "Constellation", "Rise", "Peak", "Alt", "Dir", "Set"}
	rows := make([]tableRow, len(results))

	for i, r := range results {
		name := r.Target.Name
		if name == "" {
			name = r.Target.ID
		}

		rows[i] = tableRow{
			num:           strconv.Itoa(i + 1),
			object:        name,
			kind:          string(r.Target.Kind),
			mag:           fmt.Sprintf("%+.1f", r.ApparentMag),
			constellation: r.Constellation,
			rise:          clock(r.RiseTime),
			peak:          clock(r.PeakTime),
			alt:           fmt.Sprintf("%+.0f°", r.PeakAltitude.Degrees()),
			dir:           r.Direction,
			set:           clock(r.SetTime),
			kindRaw:       r.Target.Kind,
			magVal:        r.ApparentMag,
			note:          r.SkyNote,
		}
	}

	col := func(header string, get func(tableRow) string) int {
		w := len([]rune(header))
		for _, row := range rows {
			if n := len([]rune(get(row))); n > w {
				w = n
			}
		}

		return w
	}

	getters := []func(tableRow) string{
		func(r tableRow) string { return r.num },
		func(r tableRow) string { return r.object },
		func(r tableRow) string { return r.kind },
		func(r tableRow) string { return r.mag },
		func(r tableRow) string { return r.constellation },
		func(r tableRow) string { return r.rise },
		func(r tableRow) string { return r.peak },
		func(r tableRow) string { return r.alt },
		func(r tableRow) string { return r.dir },
		func(r tableRow) string { return r.set },
	}

	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = col(h, getters[i])
	}

	border := func(left, mid, right string) string {
		var b strings.Builder

		b.WriteString(left)

		for i, w := range widths {
			b.WriteString(strings.Repeat("─", w+2))

			if i < len(widths)-1 {
				b.WriteString(mid)
			}
		}

		b.WriteString(right)

		return b.String()
	}

	pad := func(s string, w int) string {
		return s + strings.Repeat(" ", w-len([]rune(s)))
	}

	fmt.Println()
	fmt.Println(colorize(ansiBold, fmt.Sprintf("%d object(s) visible tonight, brightest first", len(results))))
	fmt.Println(colorize(ansiDim, "rise/set show — when that event is outside tonight's dusk-dawn window"))
	fmt.Println()
	fmt.Println(border("┌", "┬", "┐"))

	var headerLine strings.Builder

	headerLine.WriteString("│")

	for i, h := range headers {
		headerLine.WriteString(" ")
		headerLine.WriteString(colorize(ansiBold, pad(h, widths[i])))
		headerLine.WriteString(" │")
	}

	fmt.Println(headerLine.String())
	fmt.Println(border("├", "┼", "┤"))

	var notes []tableRow

	for _, r := range rows {
		var line strings.Builder

		line.WriteString("│ ")
		line.WriteString(pad(r.num, widths[0]))
		line.WriteString(" │ ")
		line.WriteString(colorize(kindColor(r.kindRaw), pad(r.object, widths[1])))
		line.WriteString(" │ ")
		line.WriteString(colorize(kindColor(r.kindRaw), pad(r.kind, widths[2])))
		line.WriteString(" │ ")
		line.WriteString(colorize(magColor(r.magVal), pad(r.mag, widths[3])))
		line.WriteString(" │ ")
		line.WriteString(pad(r.constellation, widths[4]))
		line.WriteString(" │ ")
		line.WriteString(colorize(ansiDim, pad(r.rise, widths[5])))
		line.WriteString(" │ ")
		line.WriteString(colorize(ansiBYellow, pad(r.peak, widths[6])))
		line.WriteString(" │ ")
		line.WriteString(pad(r.alt, widths[7]))
		line.WriteString(" │ ")
		line.WriteString(colorize(ansiCyan, pad(r.dir, widths[8])))
		line.WriteString(" │ ")
		line.WriteString(colorize(ansiDim, pad(r.set, widths[9])))
		line.WriteString(" │")

		if r.note != "" {
			line.WriteString(colorize(ansiDim, fmt.Sprintf("  [%d]", len(notes)+1)))

			notes = append(notes, r)
		}

		fmt.Println(line.String())
	}

	fmt.Println(border("└", "┴", "┘"))

	for i, r := range notes {
		fmt.Println(colorize(ansiDim, fmt.Sprintf("  [%d] %s: %s", i+1, r.object, r.note)))
	}
}
