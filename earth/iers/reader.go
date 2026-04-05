package iers

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/TuSKan/astrogo/earth"
)

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

var _ earth.Model = (*Table)(nil)

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
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].MJD < records[j].MJD
	})

	return &Table{records: records}, nil
}

// EOP returns interpolated parameters for the given Modified Julian Date.
func (t *Table) EOP(mjd float64) (earth.EOP, error) {
	if len(t.records) == 0 {
		return earth.EOP{}, fmt.Errorf("no EOP records available")
	}

	// Binary search
	i := sort.Search(len(t.records), func(i int) bool {
		return t.records[i].MJD >= mjd
	})

	if i == 0 {
		r := t.records[0]
		return earth.EOP{DUT1: r.DUT1, XP: r.XP, YP: r.YP, LOD: r.LOD}, nil
	}
	if i == len(t.records) {
		r := t.records[len(t.records)-1]
		return earth.EOP{DUT1: r.DUT1, XP: r.XP, YP: r.YP, LOD: r.LOD}, nil
	}

	r0 := t.records[i-1]
	r1 := t.records[i]

	f := (mjd - r0.MJD) / (r1.MJD - r0.MJD)
	if f < 0 { f = 0 }
	if f > 1 { f = 1 }

	return earth.EOP{
		DUT1: r0.DUT1 + f*(r1.DUT1-r0.DUT1),
		XP:   r0.XP + f*(r1.XP-r0.XP),
		YP:   r0.YP + f*(r1.YP-r0.YP),
		LOD:  r0.LOD + f*(r1.LOD-r0.LOD),
	}, nil
}
