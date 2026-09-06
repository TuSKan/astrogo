//go:build validation

package satellite

// This file verifies astrogo's satellite propagation against the reference
// vectors published with Vallado et al. (2006), "Revisiting Spacetrack
// Report #3" -- the suite every SGP4 implementation is measured by.
//
// See testdata/vallado/README.md for the data's provenance and its two traps.
// The tests are in-package because the quantity being verified is the raw TEME
// state (propagateECI), before the TEME->GCRS rotation and the km->AU scaling
// that Satellite.State applies; comparing after those would fold coord's
// accuracy into a result meant to be about SGP4 alone.

import (
	"bufio"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/time"
)

// verifiedMaxKm is the contract for the cases astrogo actually agrees on.
//
// It is deliberately NOT the measured maximum. Setting a contract to what was
// measured makes it unable to detect any regression it did not already permit;
// the measured distribution is reported separately below, and it is that
// report — not this number — which says how well the code does today.
//
// 1 km is chosen because SGP4's own intrinsic accuracy is of that order at
// epoch and degrades by kilometres per day thereafter (Vallado 2006), so a
// tighter bound would be asserting agreement finer than the model it is
// agreeing with. It also sits three orders of magnitude below the divergence
// the excluded cases show, so it separates "a correct port" from "not one"
// without tripping on floating-point noise or the whole-second API below.
const verifiedMaxKm = 1.0

// verifiedP99Km is a regression detector rather than a contract: it has no
// physical justification, only a measured one. p99 was 0.289 km when this was
// written, so this fails if the typical case degrades by ~40% while still
// passing verifiedMaxKm. Raise it only alongside an explanation of what got
// worse and why that is acceptable.
const verifiedP99Km = 0.4

// residualFloorKm is why the contract is not tighter still.
//
// The backend exposes propagation only as Propagate(sat, y, m, d, h, min, sec)
// — whole seconds — and builds its own jdsatepoch with JDay(..., int(sec)),
// truncating the element epoch to a whole second as well. propagateECI corrects
// the resulting offset to first order along the velocity vector, which leaves
// the second-order term (about 5 m for a low Earth orbit over half a second)
// plus whatever the truncated epoch does to sgp4init's derived quantities.
// Measured, that residual is tens of metres. It is a property of the API the
// backend offers, not of SGP4.
const residualFloorKm = 0.005

// divergent records the cases where astrogo does NOT reproduce the reference,
// with the magnitude measured on 2026-09-05 and Vallado's own label for what
// the case is testing.
//
// # These are not tolerances. They are a defect, written down.
//
// Every one of these is exact at tsince = 0 and grows quadratically with time
// — the signature of a wrong secular drag term, not a wrong initial state and
// not a time-handling error in this package. They cluster on precisely the
// paths Vallado built the suite to exercise: the low-perigee s4 modification,
// the deep-space SDP4 branch, and satellites in the last stage of decay.
//
// The backend's own test suite covers six of these 33 cases (5, 4632, 6251,
// 88888, 24208, 23599) and not one of the eight below, which is how a port
// with this defect passes its own verification.
//
// Each bound is checked from BOTH sides. The upper bound catches the defect
// getting worse. The lower bound matters just as much: if the propagator is
// ever fixed or replaced, the case stops diverging, this test fails, and
// whoever did it is told to move the satellite into the verified set rather
// than leaving a stale exclusion behind.
var divergent = map[string]struct {
	maxKm  float64
	reason string
}{
	"28350": {3440.27, "near-Earth, perigee 127 km — the low-perigee s4 modification"},
	"22312": {1830.30, "SL-6 R/B(2), the last element set before it decayed in 2006"},
	"16925": {1329.38, "the s4 > 20 modification"},
	"11801": {782.18, "the original Spacetrack Report #3 deep-space (SDP4) case"},
	"28623": {486.20, "H-2 R/B — deep space AND perigee 136 km, both s4 paths at once"},
	"28872": {7.73, "perigee is negative (−51 km); Vallado notes it is lost within 50 minutes"},
	"23333": {3.11, "WIND — Vallado notes the STR#3 Kepler solver fails past about 200 minutes"},
	"29141": {0.62, "SL-14 DEB in the last stage of decay, lost inside 420 minutes"},
}

// checksumInvalid are the three cases astrogo refuses before propagating.
//
// Vallado hand-built them to exercise SGP4's error returns and did not maintain
// their check digits; five of their six lines carry a checksum that does not
// match (see testdata/vallado/README.md, independently recomputed). ValidateTLE
// therefore rejects them, which is correct behaviour on data that really is
// malformed, and the reason astrogo's coverage of the suite is 30 cases and not
// 33. Asserted explicitly rather than skipped, so the number cannot quietly
// change.
var checksumInvalid = map[string]bool{"33333": true, "33334": true, "33335": true}

type refBlock struct {
	satnum string
	rows   [][7]float64 // tsince, x, y, z, vx, vy, vz
}

// loadValladoTLEs reads the element sets, keyed by satellite number with
// leading zeros stripped — the reference output writes "5" where the element
// set writes "00005", and matching them literally drops the first six cases
// while looking like it worked.
//
// One satellite (20413) appears twice with different time spans, so the value
// is a slice and the reference blocks are consumed in file order.
func loadValladoTLEs(t *testing.T) map[string][][2]string {
	t.Helper()

	f, err := os.Open("testdata/vallado/SGP4-VER.TLE")
	if err != nil {
		t.Fatalf("open element sets: %v", err)
	}
	defer func() { _ = f.Close() }()

	out := make(map[string][][2]string)

	var line1 string

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		// The file is CRLF and every line 2 carries three extra trailing
		// fields (start/stop/step in minutes) past the 69 standard columns.
		line := strings.TrimRight(sc.Text(), "\r\n ")

		switch {
		case strings.HasPrefix(line, "1 "):
			line1 = line
		case strings.HasPrefix(line, "2 "):
			num := strings.TrimLeft(strings.TrimSpace(line[2:7]), "0")
			out[num] = append(out[num], [2]string{line1, line})
		}
	}

	if err := sc.Err(); err != nil {
		t.Fatalf("read element sets: %v", err)
	}

	return out
}

// loadValladoReference reads the expected states, in file order.
func loadValladoReference(t *testing.T) []refBlock {
	t.Helper()

	f, err := os.Open("testdata/vallado/tcppver.out")
	if err != nil {
		t.Fatalf("open reference states: %v", err)
	}
	defer func() { _ = f.Close() }()

	var (
		blocks []refBlock
		cur    *refBlock
	)

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}

		if num, isHeader := strings.CutSuffix(line, " xx"); isHeader {
			blocks = append(blocks, refBlock{satnum: strings.TrimSpace(num)})
			cur = &blocks[len(blocks)-1]

			continue
		}

		if cur == nil {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 7 {
			continue
		}

		var row [7]float64

		ok := true

		for i := range row {
			v, err := strconv.ParseFloat(fields[i], 64)
			if err != nil {
				ok = false

				break
			}

			row[i] = v
		}

		if ok {
			cur.rows = append(cur.rows, row)
		}
	}

	if err := sc.Err(); err != nil {
		t.Fatalf("read reference states: %v", err)
	}

	return blocks
}

func norm3(ax, ay, az, bx, by, bz float64) float64 {
	dx, dy, dz := ax-bx, ay-by, az-bz

	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

func quantile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return math.NaN()
	}

	return sorted[int(p*float64(len(sorted)-1))]
}

// TestSGP4AgreesWithValladoReferenceVectors is the answer to "is the
// propagator correct", asked of the only source that can answer it.
//
// It is deliberately not a pass/fail on the suite as a whole. Twenty-two of
// the thirty cases astrogo can read agree to within a few hundred metres;
// eight do not, by up to 3440 km, and both facts are asserted. A single
// tolerance loose enough to cover all thirty would be 3500 km and would prove
// nothing at all.
func TestSGP4AgreesWithValladoReferenceVectors(t *testing.T) {
	t.Parallel()

	tles := loadValladoTLEs(t)
	blocks := loadValladoReference(t)

	if len(blocks) != 33 {
		t.Fatalf("reference carries %d cases, want 33 — the fixture changed", len(blocks))
	}

	var (
		verifiedPos []float64
		verifiedVel []float64
		worstOf     = make(map[string]float64)
		rejected    []string
		consumed    = make(map[string]int)
		rows        int
	)

	for _, blk := range blocks {
		sets := tles[blk.satnum]

		idx := consumed[blk.satnum]
		consumed[blk.satnum]++

		if idx >= len(sets) {
			t.Errorf("reference block for satellite %s has no element set (case %d of %d)",
				blk.satnum, idx+1, len(sets))

			continue
		}

		// Trim the three trailing start/stop/step fields off line 2.
		line1, line2 := sets[idx][0][:tleLineLength], sets[idx][1][:tleLineLength]

		sat, err := NewFromTLE(blk.satnum, line1, line2)
		if err != nil {
			rejected = append(rejected, blk.satnum)

			if !checksumInvalid[blk.satnum] {
				t.Errorf("satellite %s: NewFromTLE refused a case it should accept: %v", blk.satnum, err)
			}

			continue
		}

		if checksumInvalid[blk.satnum] {
			t.Errorf("satellite %s: NewFromTLE accepted an element set whose checksum is wrong.\n"+
				"  Five lines in this fixture carry a bad check digit; refusing them is the point.", blk.satnum)
		}

		for _, row := range blk.rows {
			at := sat.epoch.Add(time.Duration(row[0] * float64(time.Minute)))

			pos, vel, err := sat.propagateECI(at)
			if err != nil {
				t.Errorf("satellite %s at tsince %.2f: %v", blk.satnum, row[0], err)

				continue
			}

			dp := norm3(pos.X, pos.Y, pos.Z, row[1], row[2], row[3])
			dv := norm3(vel.X, vel.Y, vel.Z, row[4], row[5], row[6])
			rows++

			if dp > worstOf[blk.satnum] {
				worstOf[blk.satnum] = dp
			}

			if _, known := divergent[blk.satnum]; !known {
				verifiedPos = append(verifiedPos, dp)
				verifiedVel = append(verifiedVel, dv)

				if dp > verifiedMaxKm {
					t.Errorf("satellite %s at tsince %.2f: position differs from Vallado by %.4f km, contract is %.1f km.\n"+
						"  This case is in the verified set, so this is a regression, not a known limitation.",
						blk.satnum, row[0], dp, verifiedMaxKm)
				}
			}
		}
	}

	// Every divergent case must still diverge by roughly what was recorded —
	// checked from both sides, so a fix is reported as loudly as a regression.
	for num, want := range divergent {
		got, ran := worstOf[num]
		if !ran {
			t.Errorf("satellite %s is recorded as divergent but never ran", num)

			continue
		}

		switch {
		case got > want.maxKm*2:
			t.Errorf("satellite %s (%s) now differs by %.2f km, was %.2f km — the defect got worse",
				num, want.reason, got, want.maxKm)
		case got < want.maxKm/2:
			t.Errorf("satellite %s (%s) now differs by only %.4f km, was %.2f km.\n"+
				"  If the propagator was fixed or replaced, move this case into the verified set "+
				"and record the new figure — a stale exclusion hides the next regression.",
				num, want.reason, got, want.maxKm)
		}
	}

	if len(rejected) != len(checksumInvalid) {
		t.Errorf("astrogo refused %v; the only cases it should refuse are the %d with bad checksums",
			rejected, len(checksumInvalid))
	}

	sort.Float64s(verifiedPos)
	sort.Float64s(verifiedVel)

	if p99 := quantile(verifiedPos, 0.99); p99 > verifiedP99Km {
		t.Errorf("verified-set p99 position error is %.4f km, regression detector is %.2f km.\n"+
			"  Still inside the %.1f km contract, but the typical case has degraded.", p99, verifiedP99Km, verifiedMaxKm)
	}

	// The floor: if agreement suddenly became perfect, the comparison has
	// stopped comparing — a fixture read as all zeros, or a loop that never
	// ran. Given the whole-second API this cannot legitimately reach zero.
	if p50 := quantile(verifiedPos, 0.5); p50 < residualFloorKm/10 {
		t.Errorf("verified-set median error is %.9g km, which is below anything the whole-second "+
			"backend API can produce. The comparison is probably not comparing.", p50)
	}

	t.Logf("compared %d states across %d cases (%d refused for bad checksums)", rows, len(blocks)-len(rejected), len(rejected))
	t.Logf("verified set: n=%d  p50=%.4f  p90=%.4f  p99=%.4f  max=%.4f km",
		len(verifiedPos), quantile(verifiedPos, .5), quantile(verifiedPos, .9),
		quantile(verifiedPos, .99), quantile(verifiedPos, 1))
	t.Logf("verified set: velocity p50=%.3g  max=%.3g km/s",
		quantile(verifiedVel, .5), quantile(verifiedVel, 1))

	type reported struct {
		num string
		km  float64
	}

	worst := make([]reported, 0, len(divergent))

	for num := range divergent {
		worst = append(worst, reported{num, worstOf[num]})
	}

	sort.Slice(worst, func(i, j int) bool { return worst[i].km > worst[j].km })

	for _, w := range worst {
		t.Logf("DIVERGENT sat %-6s max %10.2f km  (%s)", w.num, w.km, divergent[w.num].reason)
	}
}

// TestSubSecondCorrectionIsMeasuredFromTheEpoch is the regression test for the
// bug the Vallado suite exposed, isolated so a failure says what broke.
//
// propagateECI corrected the whole-second truncation by frac(t), but the
// backend truncates the element epoch too, so the correction has to be
// frac(t) - frac(epoch). Using frac(t) alone left a fixed error of one
// element set's worth of sub-second motion — 5.94 km here — and it was worst
// at the epoch itself, where the answer should be exact.
func TestSubSecondCorrectionIsMeasuredFromTheEpoch(t *testing.T) {
	t.Parallel()

	// Vallado's satellite 5, whose epoch falls 0.7336 s past a whole second.
	const (
		line1 = "1 00005U 58002B   00179.78495062  .00000023  00000-0  28098-4 0  4753"
		line2 = "2 00005  34.2682 348.7242 1859667 331.7664  19.3264 10.82419157413667"
	)

	sat, err := NewFromTLE("5", line1, line2)
	if err != nil {
		t.Fatalf("NewFromTLE: %v", err)
	}

	if sat.epochFracSec < 0.7 || sat.epochFracSec > 0.77 {
		t.Fatalf("precondition: epochFracSec = %v, want ~0.7336 — this element set was chosen "+
			"because its epoch is nearly three quarters of a second past the whole second, "+
			"which is what makes the bug visible", sat.epochFracSec)
	}

	// tsince = 0: the state at the epoch, where the reference is the element
	// set itself and any error is purely the correction being misapplied.
	wantX, wantY, wantZ := 7022.46529266, -1400.08296755, 0.03995155

	pos, _, err := sat.propagateECI(sat.epoch)
	if err != nil {
		t.Fatalf("propagateECI: %v", err)
	}

	dp := norm3(pos.X, pos.Y, pos.Z, wantX, wantY, wantZ)
	if dp > 0.1 {
		t.Errorf("at the element epoch the position differs from Vallado by %.4f km.\n"+
			"  Measuring the sub-second correction from zero instead of from the epoch's own "+
			"fractional second puts it at 5.94 km here; every element set gets its own fixed "+
			"offset of up to a second of motion, which is 7.5 km in low Earth orbit.", dp)
	}

	if dp < 1e-9 {
		t.Errorf("agreement of %.3g km is closer than the backend's whole-second API allows; "+
			"the comparison is probably not comparing", dp)
	}

	t.Logf("position at epoch differs from Vallado by %.1f m", dp*1000)
}

// TestValladoFixtureShapeIsWhatTheSuiteAssumes fails early and clearly when
// the checked-in data changes, rather than letting the suite above degrade
// into comparing nothing.
func TestValladoFixtureShapeIsWhatTheSuiteAssumes(t *testing.T) {
	t.Parallel()

	tles := loadValladoTLEs(t)
	blocks := loadValladoReference(t)

	if len(tles) != 32 {
		t.Errorf("element sets cover %d satellites, want 32 (33 cases, 20413 appearing twice)", len(tles))
	}

	total := 0
	for _, sets := range tles {
		total += len(sets)
	}

	if total != 33 {
		t.Errorf("element sets total %d, want 33", total)
	}

	if len(blocks) != 33 {
		t.Errorf("reference carries %d blocks, want 33", len(blocks))
	}

	// The zero-padding trap: if these ever match literally, the "5" vs "00005"
	// note in the README is stale and loadValladoTLEs' TrimLeft is load-bearing
	// for a reason that no longer exists.
	if _, ok := tles["00005"]; ok {
		t.Error("element sets are keyed with leading zeros; the reference output is not")
	}

	for _, blk := range blocks {
		if len(blk.rows) == 0 {
			t.Errorf("reference block %s carries no states", blk.satnum)
		}

		for _, row := range blk.rows {
			r := math.Sqrt(row[1]*row[1] + row[2]*row[2] + row[3]*row[3])
			if r < 6000 || r > 1e6 {
				t.Errorf("%s at tsince %.2f: |r| = %.1f km is not a plausible geocentric distance",
					blk.satnum, row[0], r)
			}
		}
	}

	t.Logf("%d cases, %d element sets, %d satellites", len(blocks), total, len(tles))
}
