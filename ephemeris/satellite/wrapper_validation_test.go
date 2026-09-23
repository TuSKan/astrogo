//go:build validation

package satellite

import (
	"bufio"
	"errors"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/satellite/sgp4"
	"github.com/TuSKan/astrogo/unit"
)

// This file used to be 530 lines comparing astrogo's propagation against
// Vallado's reference states, because that comparison had nowhere else to live.
// It lives in ephemeris/satellite/sgp4 now, against the same fixture, at a
// contract four orders tighter (1e-4 km against the 1 km this file asserted),
// covering 33 cases where this covered 30, and running untagged on every build
// rather than only under the validation tier.
//
// What is left here is the question this layer can answer and that one cannot:
// does the wrapper hand back what the model produced?

// wrapperMaxKM is deliberately loose. It is not an accuracy claim — sgp4's own
// suite makes that one — it is a bound on transport: the number coming out of
// Satellite.propagateECI must be the number sgp4 put in, and a meter sits far
// below anything a unit slip, a frame confusion or a dropped epoch fraction
// could hide inside.
const wrapperMaxKM = 1e-3

// TestTheWrapperDoesNotCorruptTheModel propagates the reference cases through
// Satellite rather than through sgp4 directly.
//
// # Why this is worth its own test
//
// Because the wrapper is where astrogo's own mistakes have always been. Not one
// of the defects this package has carried was in the SGP4 arithmetic: the
// gravity model was passed wrong (93x the position error), the epoch fraction
// was corrected from the wrong origin (5.94 km), and the element set was parsed
// twice in two places that could disagree. Every one lived between the caller
// and the model, and every one would be invisible to a test that calls the
// model directly.
func TestTheWrapperDoesNotCorruptTheModel(t *testing.T) {
	t.Parallel()

	sets := loadValladoTLEs(t)
	blocks := loadValladoReference(t)

	used := make(map[string]int)

	var (
		compared, checksumRefusals int
		worst                      float64
		worstAt                    string
	)

	for _, blk := range blocks {
		idx := used[blk.satnum]
		used[blk.satnum]++

		candidates := sets[blk.satnum]
		if idx >= len(candidates) {
			continue
		}

		sat, err := NewFromTLE(blk.satnum,
			candidates[idx][0][:sgp4.LineLength], candidates[idx][1][:sgp4.LineLength])
		if err != nil {
			// The three cases with unmaintained check digits are refused here
			// and not by sgp4, which is the point of the split: this layer
			// reads network feeds, so it enforces the check digit.
			if errors.Is(err, sgp4.ErrChecksum) {
				checksumRefusals++

				continue
			}

			t.Fatalf("satellite %s: NewFromTLE: %v", blk.satnum, err)
		}

		for _, row := range blk.rows {
			// The reference's own argument, converted the way a caller would.
			// No state in the reference errors — measured over every row —
			// so an error here is the wrapper's, not the model's.
			pos, _, perr := sat.propagateECI(sat.epoch.Add(unit.Days(row[0] / 1440.0)))
			if perr != nil {
				t.Errorf("satellite %s at tsince %g: %v", blk.satnum, row[0], perr)

				continue
			}

			d := norm3(pos.X, pos.Y, pos.Z, row[1], row[2], row[3])
			compared++

			if d > worst {
				worst, worstAt = d, blk.satnum
			}

			if d > wrapperMaxKM {
				t.Errorf("satellite %s at tsince %g: the wrapper produced a position %.6g km "+
					"from the reference, against a %.0e km transport bound.\n"+
					"  sgp4 agrees with this same fixture to 4.1e-06 km, so a gap this size "+
					"is something the wrapper did: a unit, a frame, or the epoch.",
					blk.satnum, row[0], d, wrapperMaxKM)
			}
		}
	}

	if compared == 0 {
		t.Fatal("no states were compared; the fixtures are not being read")
	}

	if checksumRefusals != 3 {
		t.Errorf("%d element sets refused for their check digits, want the fixture's 3", checksumRefusals)
	}

	t.Logf("%d states through Satellite.propagateECI, worst %.4g km (satellite %s)",
		compared, worst, worstAt)
}

// TestTheEpochFractionSurvivesTheWrapper is the regression test for the defect
// this layer used to carry, kept although the code that caused it is gone.
//
// The old backend took whole seconds and truncated the element epoch the same
// way, so the wrapper corrected by frac(t) − frac(epoch). Getting that origin
// wrong left a fixed offset per element set — worst AT the epoch, where the
// answer should be exact — and measured 5.94 km on Vallado's satellite 5.
//
// There is no correction now. This asserts the property the correction existed
// to provide: at tsince 0 the wrapper reproduces the reference, whatever the
// sub-second part of the epoch happens to be. The count check at the end is
// what stops it passing vacuously — an element set whose epoch lands on a whole
// second could never have shown the defect, so it proves nothing.
func TestTheEpochFractionSurvivesTheWrapper(t *testing.T) {
	t.Parallel()

	sets := loadValladoTLEs(t)
	blocks := loadValladoReference(t)

	checked := 0

	for _, blk := range blocks {
		ss, ok := sets[blk.satnum]
		if !ok || len(ss) == 0 || len(blk.rows) == 0 || blk.rows[0][0] != 0 {
			continue
		}

		sat, err := NewFromTLE(blk.satnum, ss[0][0][:sgp4.LineLength], ss[0][1][:sgp4.LineLength])
		if err != nil {
			if errors.Is(err, sgp4.ErrChecksum) {
				continue
			}

			t.Fatalf("satellite %s: NewFromTLE: %v", blk.satnum, err)
		}

		if frac := math.Abs(math.Mod(sat.epoch.JD()*86400.0, 1.0)); frac < 1e-6 || frac > 1-1e-6 {
			continue
		}

		pos, _, err := sat.propagateECI(sat.epoch)
		if err != nil {
			t.Errorf("satellite %s at its own epoch: %v", blk.satnum, err)

			continue
		}

		row := blk.rows[0]
		checked++

		if d := norm3(pos.X, pos.Y, pos.Z, row[1], row[2], row[3]); d > wrapperMaxKM {
			t.Errorf("satellite %s at its own epoch is %.6g km from the reference. The "+
				"sub-second part of the epoch is being lost between NewFromTLE and the "+
				"propagator.", blk.satnum, d)
		}
	}

	if checked < 10 {
		t.Errorf("only %d element sets with a sub-second epoch were checked; this test "+
			"proves nothing about a defect only such sets can show", checked)
	}
}

func norm3(ax, ay, az, bx, by, bz float64) float64 {
	dx, dy, dz := ax-bx, ay-by, az-bz

	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// loadValladoTLEs reads the element sets, keyed by satellite number with
// leading zeros stripped — the reference output writes "5" where the element
// set writes "00005", and matching them literally drops the first six cases
// while looking like it worked.
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

type refBlock struct {
	satnum string
	rows   [][7]float64 // tsince, x, y, z, vx, vy, vz
}

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

		var (
			row [7]float64
			ok  = true
		)

		for i := range row {
			v, perr := strconv.ParseFloat(fields[i], 64)
			if perr != nil {
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
