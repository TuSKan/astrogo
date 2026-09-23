package mpcorb

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// Column bounds of the MPC's comet format, as half-open Go slice indices —
// the format CometEls.txt is written in, which the MPC's "Export Format for
// Comet Orbits" documents as the one used in "Ephemerides and Orbital
// Elements" (www.minorplanetcenter.net/iau/info/CometOrbitFormat.html, the
// second of its two tables; the first is the ECS format, laid out
// differently). The comments give the MPC's own 1-based columns.
const (
	cometIDEnd         = 12  //   1-12   number, orbit type, packed designation
	cometTypeCol       = 4   //   5      orbit type: C, P, D, A, I or X
	cometTYearStart    = 14  //  15-18   year of perihelion passage
	cometTYearEnd      = 18  //
	cometTMonthStart   = 19  //  20-21   month of perihelion passage
	cometTMonthEnd     = 21  //
	cometTDayStart     = 22  //  23-29   day of perihelion passage, TT
	cometTDayEnd       = 29  //
	cometQStart        = 30  //  31-39   perihelion distance, AU
	cometQEnd          = 39  //
	cometEccStart      = 41  //  42-49   eccentricity
	cometEccEnd        = 49  //
	cometPeriStart     = 51  //  52-59   argument of perihelion, degrees, J2000
	cometPeriEnd       = 59  //
	cometNodeStart     = 61  //  62-69   longitude of ascending node, degrees, J2000
	cometNodeEnd       = 69  //
	cometInclStart     = 71  //  72-79   inclination, degrees, J2000
	cometInclEnd       = 79  //
	cometEpochStart    = 81  //  82-89   epoch, YYYYMMDD, 0h TT; blank if unperturbed
	cometEpochEnd      = 89  //
	cometHStart        = 91  //  92-95   absolute magnitude
	cometHEnd          = 95  //
	cometSlopeStart    = 96  //  97-100  slope parameter
	cometSlopeEnd      = 100 //
	cometNameStart     = 102 // 103-158  designation and name
	cometNameEnd       = 158 //
	cometMinRowWidth   = cometSlopeEnd
	cometMaxRowWidth   = colMinRowWidth - 1 // an MPCORB row is at least this long
	cometSlopeToK1     = 2.5
	cometTypeCodes     = "CPDAIX"
	cometInterstellar  = 'I'
	cometAsteroidOrbit = 'A'
)

// isCometRow reports whether row is a comet-format element row rather than a
// header, a blank line or an MPCORB row.
//
// Structural, like isElementRow: the perihelion date's own separators, a
// known orbit type, and an eccentricity that parses. Length keeps the two
// formats apart — a comet row ends by column 172, an MPCORB row runs to
// column 194 or beyond — and a comet row read as MPCORB would put its
// inclination where the eccentricity belongs.
func isCometRow(row string) bool {
	if len(row) < cometMinRowWidth || len(row) > cometMaxRowWidth {
		return false
	}

	if !strings.ContainsRune(cometTypeCodes, rune(row[cometTypeCol])) ||
		row[cometTYearEnd] != ' ' || row[cometTMonthEnd] != ' ' {
		return false
	}

	if _, err := strconv.Atoi(row[cometTYearStart:cometTYearEnd]); err != nil {
		return false
	}

	_, err := strconv.ParseFloat(strings.TrimSpace(row[cometEccStart:cometEccEnd]), 64)

	return err == nil
}

// parseCometRow converts one comet-format row into a resolve.Target carrying
// the comet form of the orbit: perihelion distance and time, eccentricity and
// the three angles. The asteroid form is left at zero; the file does not
// publish it, and a parabola or hyperbola has none.
func parseCometRow(row string) (resolve.Target, error) {
	name := strings.TrimSpace(row[cometNameStart:min(len(row), cometNameEnd)])

	t := resolve.Target{
		// The packed number, type and designation, with the padding between
		// them removed: "0001P", "CJ95O010", and "0003Da" for a fragment.
		ID:          strings.Join(strings.Fields(row[:cometIDEnd]), ""),
		Name:        name,
		Designation: name,
		Catalog:     "mpcorb",
		Kind:        cometKind(row[cometTypeCol]),
	}

	// "C/1995 O1 (Hale-Bopp)" is designated C/1995 O1; "1P/Halley" has no
	// separate name.
	if i := strings.Index(name, " ("); i > 0 {
		t.Designation = name[:i]
	}

	tp, err := cometPerihelionTime(row)
	if err != nil {
		return resolve.Target{}, fmt.Errorf("%w: %s: %w", ErrMalformedRow, t.ID, err)
	}

	for _, f := range []struct {
		name string
		text string
		set  func(float64)
	}{
		{"perihelion distance", row[cometQStart:cometQEnd], func(v float64) { t.PerihelionDistance = unit.AU(v) }},
		{"eccentricity", row[cometEccStart:cometEccEnd], func(v float64) { t.Eccentricity = v }},
		{"inclination", row[cometInclStart:cometInclEnd], func(v float64) { t.Inclination = angle.Deg(v) }},
		{"ascending node", row[cometNodeStart:cometNodeEnd], func(v float64) { t.AscendingNode = angle.Deg(v) }},
		{"argument of perihelion", row[cometPeriStart:cometPeriEnd], func(v float64) { t.ArgPeriapsis = angle.Deg(v) }},
	} {
		v, err := strconv.ParseFloat(strings.TrimSpace(f.text), 64)
		if err != nil {
			return resolve.Target{}, fmt.Errorf("%w: %s: %s %q", ErrMalformedRow, t.ID, f.name, f.text)
		}

		f.set(v)
	}

	t.PerihelionTime = tp

	// An unperturbed solution has no epoch of osculation: it is a two-body
	// fit, defined at its perihelion, and that is the epoch it is given.
	t.Epoch = tp

	if text := strings.TrimSpace(row[cometEpochStart:cometEpochEnd]); text != "" {
		epoch, err := cometEpoch(text)
		if err != nil {
			return resolve.Target{}, fmt.Errorf("%w: %s: epoch %q: %w", ErrMalformedRow, t.ID, text, err)
		}

		t.Epoch = epoch
	}

	t.HasElements = true

	// The MPC's comet magnitude law is m = H + 5 log Δ + 2.5·G·log r, with
	// G the "slope parameter" in columns 97-100: it exports the same pair to
	// XEphem as g and k, and libastro's gk_mag computes
	// g + 5·log10(Δ) + 2.5·k·log10(r). astrogo's K1 multiplies log r
	// directly, so it is 2.5·G — the 4.0 most rows carry is the familiar
	// 10·log r.
	if h, err := strconv.ParseFloat(strings.TrimSpace(row[cometHStart:cometHEnd]), 64); err == nil {
		if g, err := strconv.ParseFloat(strings.TrimSpace(row[cometSlopeStart:cometSlopeEnd]), 64); err == nil {
			t.M1, t.K1, t.HasM1 = h, cometSlopeToK1*g, true
		}
	}

	return t, nil
}

// cometKind maps the MPC's orbit-type letter to a Kind. An A/ object is an
// asteroid on a cometary orbit and an I/ object is interstellar; C, P, D
// and X are all comets, however well observed.
func cometKind(code byte) resolve.Kind {
	switch code {
	case cometInterstellar:
		return resolve.KindInterstellar
	case cometAsteroidOrbit:
		return resolve.KindAsteroid
	default:
		return resolve.KindComet
	}
}

// errPerihelionDate is a perihelion month or day that is not one. It reaches
// a caller wrapped in ErrMalformedRow.
var errPerihelionDate = errors.New("perihelion date out of range")

// cometPerihelionTime reads the perihelion date — year, month and a day with
// four decimals, in TT. The midnight's Julian Date is formed through the UTC
// calendar and labeled TT, as ParseEpoch does, and the day's fraction is
// carried as a second part so it keeps its precision.
func cometPerihelionTime(row string) (time.Time, error) {
	year, err := strconv.Atoi(row[cometTYearStart:cometTYearEnd])
	if err != nil {
		return time.Time{}, fmt.Errorf("perihelion year: %w", err)
	}

	month, err := strconv.Atoi(strings.TrimSpace(row[cometTMonthStart:cometTMonthEnd]))
	if err != nil || month < 1 || month > 12 {
		return time.Time{}, fmt.Errorf("%w: month %q", errPerihelionDate, row[cometTMonthStart:cometTMonthEnd])
	}

	day, err := strconv.ParseFloat(strings.TrimSpace(row[cometTDayStart:cometTDayEnd]), 64)
	if err != nil || day < 1 || day >= 32 {
		return time.Time{}, fmt.Errorf("%w: day %q", errPerihelionDate, row[cometTDayStart:cometTDayEnd])
	}

	whole := math.Floor(day)
	midnight := time.Date(year, time.Month(month), int(whole), 0, 0, 0, 0, time.LocationUTC)

	return time.FromJDParts(midnight.JD(), day-whole, time.TT), nil
}

// cometEpoch reads an epoch written YYYYMMDD, at 0h TT.
func cometEpoch(text string) (time.Time, error) {
	if len(text) != 8 {
		return time.Time{}, fmt.Errorf("%w: want YYYYMMDD", ErrMalformedEpoch)
	}

	ymd, err := strconv.Atoi(text)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %w", ErrMalformedEpoch, err)
	}

	year, month, day := ymd/10000, ymd/100%100, ymd%100
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return time.Time{}, fmt.Errorf("%w: month %d day %d", ErrMalformedEpoch, month, day)
	}

	midnight := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.LocationUTC)

	return time.FromJD(midnight.JD(), time.TT), nil
}
