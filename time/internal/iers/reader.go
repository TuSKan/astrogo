package iers

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
)

// ErrOutOfRange indicates that the requested MJD falls outside
// the coverage window of the loaded IERS EOP data.
var ErrOutOfRange = errors.New("iers: MJD out of EOP data coverage")

// ErrNoRecords indicates the EOP table has no records loaded.
var ErrNoRecords = errors.New("iers: no EOP records available")

// Record is a single line of IERS EOP data.
type Record struct {
	MJD  float64
	DUT1 float64
	XP   float64
	YP   float64
	LOD  float64
}

// Table caches EOP models and interpolates them linearly by MJD.
type Table struct {
	records []Record
}

// leapJumpThreshold is how far UT1−UTC may change between consecutive daily
// records before the change is read as a leap second rather than as rotation.
//
// Earth's rotation moves UT1−UTC by a few milliseconds a day — the length-of-day
// excess, currently under 2 ms — so two adjacent records never differ by
// anything near half a second unless UTC itself stepped between them. A leap
// second steps it by exactly one. Half a second sits two orders of magnitude
// above the one and a factor of two below the other.
const leapJumpThreshold = 0.5

// dut1Across returns r1's UT1−UTC as it would read had no leap second been
// inserted since r0: that is, with any whole-second step between them removed.
//
// # Why interpolation needs it
//
// UT1 is continuous — it is Earth's rotation angle — while UTC stops for a
// leap second, so UT1−UTC jumps by exactly one second at each. The records are
// daily at 0h UTC and a leap second is inserted at the end of a UTC day, so the
// jump always falls between two records and never inside one: interpolating the
// raw values interpolates across a discontinuity.
//
// Measured on the IERS series before this existed, 2016-12-31 at 12:00 UTC
// read +0.0918 s against a true −0.4083 s, and at 23:59 read +0.5906 s against
// −0.4088 s: a ramp from one side of the step to the other, reaching a full
// second — 15 arcsec of Earth rotation — just before midnight, on every
// leap-second day in the record. See #377.
//
// With the step removed the interpolant follows UT1−UTC as it was before the
// leap, which is what it was, right up to the leap second itself. [Table.EOP]
// returns r1 unadjusted when queried exactly at r1, the one instant already on
// the far side.
//
// The step is found in the data rather than read from a ΔAT table because this
// package sits below the one that owns ΔAT, and the data says it unambiguously:
// see [leapJumpThreshold].
func dut1Across(r0, r1 Record) float64 {
	step := r1.DUT1 - r0.DUT1
	if math.Abs(step) < leapJumpThreshold {
		return r1.DUT1
	}

	return r1.DUT1 - math.Round(step)
}

var _ Model = (*Table)(nil)

// ParseFinals2000A parses IERS finals2000A.all format.
// It converts arcseconds to radians for XP and YP.
func ParseFinals2000A(r io.Reader) (*Table, error) {
	scanner := bufio.NewScanner(r)

	var records []Record

	arcsec2rad := math.Pi / (180.0 * 3600.0)

	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 86 {
			continue // skip incomplete lines or headers
		}

		mjdStr := strings.TrimSpace(line[7:15])
		if mjdStr == "" {
			continue
		}

		mjd, err := strconv.ParseFloat(mjdStr, 64)
		if err != nil {
			continue
		}

		xStr := strings.TrimSpace(line[18:27])
		yStr := strings.TrimSpace(line[37:46])
		dut1Str := strings.TrimSpace(line[58:68])
		lodStr := strings.TrimSpace(line[79:86])

		// Polar motion and UT1-UTC are what make a row an orientation. A row
		// that has an MJD and none of them is the bulletin running out, not an
		// epoch at which the pole is centered and UT1 equals UTC.
		//
		// finals2000A is padded to full width for its whole length, so those
		// rows are not short and the length check above does not reach them.
		// Parsing them with the error discarded stored fifty records of
		// exactly zero at the end of the current file - fifty days over which
		// Coverage claimed data it did not have, EOP answered instead of
		// returning ErrOutOfRange, the one-time "EOP unavailable" warning
		// therefore never fired, and the last real day was interpolated into a
		// fabricated zero. A DUT1 of zero asserts UT1 = UTC, which is wrong by
		// up to 0.9 seconds: thirteen and a half arcseconds of Earth rotation,
		// applied silently to every topocentric position in the window.
		x, err := strconv.ParseFloat(xStr, 64)
		if err != nil {
			continue
		}

		y, err := strconv.ParseFloat(yStr, 64)
		if err != nil {
			continue
		}

		dut1, err := strconv.ParseFloat(dut1Str, 64)
		if err != nil {
			continue
		}

		// Length of day is genuinely optional. It is absent from far more rows
		// than the others - hundreds rather than tens in the current file - it
		// is a rate rather than an offset, and zero is its own physical
		// default: no excess over 86400 seconds.
		lod, err := strconv.ParseFloat(lodStr, 64)
		if err != nil {
			lod = 0
		}

		records = append(records, Record{
			MJD:  mjd,
			XP:   x * arcsec2rad,
			YP:   y * arcsec2rad,
			DUT1: dut1,
			LOD:  lod,
		})
	}

	err := scanner.Err()
	if err != nil {
		return nil, fmt.Errorf("iers: scan EOP: %w", err)
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].MJD < records[j].MJD
	})

	return &Table{records: records}, nil
}

// Coverage returns the MJD range [min, max] covered by the loaded EOP data.
// Returns (0, 0) if no records are loaded.
func (t *Table) Coverage() (mjdMin, mjdMax float64) {
	if len(t.records) == 0 {
		return 0, 0
	}

	return t.records[0].MJD, t.records[len(t.records)-1].MJD
}

// EOP returns interpolated parameters for the given Modified Julian Date.
// Returns ErrOutOfRange if mjd falls outside the coverage of the loaded data.
func (t *Table) EOP(mjd float64) (EOP, error) {
	if len(t.records) == 0 {
		return EOP{}, ErrNoRecords
	}

	// Reject queries outside the data coverage window.
	if mjd < t.records[0].MJD || mjd > t.records[len(t.records)-1].MJD {
		return EOP{}, fmt.Errorf("%w: MJD %.1f not in [%.1f, %.1f]",
			ErrOutOfRange, mjd, t.records[0].MJD, t.records[len(t.records)-1].MJD)
	}

	// Binary search
	i := sort.Search(len(t.records), func(i int) bool {
		return t.records[i].MJD >= mjd
	})

	// Exact match on first record
	if i == 0 {
		r := t.records[0]
		return EOP{DUT1: r.DUT1, XP: r.XP, YP: r.YP, LOD: r.LOD}, nil
	}

	r0 := t.records[i-1]
	r1 := t.records[i]

	// A query landing exactly on a record gets that record. Not merely a
	// shortcut: the leap-second handling below adjusts r1 to be continuous
	// with r0, which is right for every instant before r1 and wrong for r1
	// itself, which is already on the far side of the step.
	if mjd == r1.MJD {
		return EOP{DUT1: r1.DUT1, XP: r1.XP, YP: r1.YP, LOD: r1.LOD}, nil
	}

	f := (mjd - r0.MJD) / (r1.MJD - r0.MJD)
	if f < 0 {
		f = 0
	}

	if f > 1 {
		f = 1
	}

	return EOP{
		DUT1: r0.DUT1 + f*(dut1Across(r0, r1)-r0.DUT1),
		XP:   r0.XP + f*(r1.XP-r0.XP),
		YP:   r0.YP + f*(r1.YP-r0.YP),
		LOD:  r0.LOD + f*(r1.LOD-r0.LOD),
	}, nil
}
