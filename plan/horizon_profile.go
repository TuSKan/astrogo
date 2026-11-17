package plan

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/TuSKan/astrogo/angle"
)

// ErrHorizonProfile is returned for a horizon table ParseHorizonProfile
// cannot read: a line that is not an azimuth and an altitude, a value out of
// range, or no point at all.
var ErrHorizonProfile = errors.New("plan: horizon profile")

// maxHorizonProfileLine bounds a line of a horizon table, so a file with no
// newline in it cannot make the reader buffer the whole of it.
const maxHorizonProfileLine = 64 << 10

// ParseHorizonProfile reads a horizon table, the local horizon's altitude
// against azimuth, as observers keep it, into a [HorizonProfile] for
// [WithHorizonProfile].
//
// Each line is one point: an azimuth and an altitude in degrees, separated by
// spaces, tabs or a comma, the azimuth from north through east. Anything
// after them on the line is ignored, as is a line that is blank or starts
// with "#" or ";". This is the form Stellarium's polygonal landscapes, APT
// and NINA write. An azimuth may be given outside [0°, 360°) and is wrapped;
// an altitude must lie in [−90°, 90°].
//
// Between points the altitude is interpolated linearly in azimuth, across
// north as well, from the last point to the first. One point is a level
// horizon at its altitude. Two points at one azimuth, in the table's order,
// make a step, as at a wall's edge: the horizon reaches the first's altitude
// there and leaves from the second's.
//
// The reader is held to never panicking or hanging on any input
// (FuzzParseHorizonProfile), since a horizon file is the caller's or a third
// party's.
func ParseHorizonProfile(r io.Reader) (HorizonProfile, error) {
	type point struct{ az, alt float64 }

	var pts []point

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 4096), maxHorizonProfileLine)

	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		f := strings.FieldsFunc(line, func(r rune) bool { return r == ' ' || r == '\t' || r == ',' })
		if len(f) < 2 {
			return nil, fmt.Errorf("%w: line %d: want an azimuth and an altitude, got %q", ErrHorizonProfile, n, line)
		}

		az, err1 := strconv.ParseFloat(f[0], 64)
		alt, err2 := strconv.ParseFloat(f[1], 64)

		switch {
		case err1 != nil || err2 != nil:
			return nil, fmt.Errorf("%w: line %d: not two numbers: %q", ErrHorizonProfile, n, line)
		case math.IsNaN(az) || math.IsInf(az, 0):
			return nil, fmt.Errorf("%w: line %d: azimuth %v", ErrHorizonProfile, n, az)
		case !(alt >= -90 && alt <= 90):
			return nil, fmt.Errorf("%w: line %d: altitude %v outside [-90, 90]", ErrHorizonProfile, n, alt)
		}

		pts = append(pts, point{az: wrap360(az), alt: alt})
	}

	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrHorizonProfile, err)
	}

	if len(pts) == 0 {
		return nil, fmt.Errorf("%w: no points", ErrHorizonProfile)
	}

	// Sorted by azimuth, and stably, so two points at one azimuth keep their
	// order in the table and make a step: the search below interpolates up to
	// the first and away from the second.
	slices.SortStableFunc(pts, func(a, b point) int { return cmpFloat(a.az, b.az) })

	return func(azimuth angle.Angle) angle.Angle {
		if len(pts) == 1 {
			return angle.Deg(pts[0].alt)
		}

		az := wrap360(azimuth.Degrees())

		// The first point at or past az, and the one before it, across north.
		i, _ := slices.BinarySearchFunc(pts, az, func(p point, az float64) int { return cmpFloat(p.az, az) })

		hi := pts[i%len(pts)]
		lo := pts[(i+len(pts)-1)%len(pts)]

		span := hi.az - lo.az
		if span <= 0 {
			span += 360
		}

		into := az - lo.az
		if into < 0 {
			into += 360
		}

		return angle.Deg(lo.alt + (hi.alt-lo.alt)*into/span)
	}, nil
}

// wrap360 returns deg wrapped into [0, 360).
func wrap360(deg float64) float64 {
	deg = math.Mod(deg, 360)
	if deg < 0 {
		deg += 360
	}

	// math.Mod(-1e-17, 360) + 360 rounds to 360.
	if deg >= 360 {
		deg = 0
	}

	return deg
}

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
