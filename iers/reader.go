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

		x, _ := strconv.ParseFloat(xStr, 64)
		y, _ := strconv.ParseFloat(yStr, 64)
		dut1, _ := strconv.ParseFloat(dut1Str, 64)
		lod, _ := strconv.ParseFloat(lodStr, 64)

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
		return nil, err
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
		return EOP{}, errors.New("no EOP records available")
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

	f := (mjd - r0.MJD) / (r1.MJD - r0.MJD)
	if f < 0 {
		f = 0
	}

	if f > 1 {
		f = 1
	}

	return EOP{
		DUT1: r0.DUT1 + f*(r1.DUT1-r0.DUT1),
		XP:   r0.XP + f*(r1.XP-r0.XP),
		YP:   r0.YP + f*(r1.YP-r0.YP),
		LOD:  r0.LOD + f*(r1.LOD-r0.LOD),
	}, nil
}
