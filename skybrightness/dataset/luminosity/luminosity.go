// Package luminosity supplies the CIE luminous efficiency functions, which
// are what turn a spectral radiance into a luminance the human eye would
// report.
//
// # Why this is a dataset rather than a formula
//
// Because V(lambda) and V'(lambda) are measurements of the human eye. They
// were established by matching experiments on real observers and there is no
// expression to derive them from — which is exactly why this module could
// report a sky in mag/arcsec^2, in photons and in detector electrons but not
// in cd/m^2 until they arrived.
//
// The two curves describe different receptors and are not interchangeable.
// Photopic vision is the cones, peaking at 555 nm; scotopic vision is the
// rods, peaking at 507 nm and blind to the red end. A night sky is
// overwhelmingly a scotopic problem, which is the whole reason a sodium
// street lamp and a blue-rich LED of equal photopic output do not look
// equally bright to a dark-adapted observer.
package luminosity

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/unit"
)

// ErrCurve is returned when a luminous efficiency function cannot be
// resolved.
var ErrCurve = errors.New("luminosity: curve")

// Vision selects which receptor's luminous efficiency function to read.
type Vision int

// The two CIE luminous efficiency functions.
const (
	// Photopic is the CIE 1924 V(lambda) for cone vision, 360-830 nm,
	// peaking at 555 nm. It is the function every lux meter and every
	// lighting standard is defined against.
	Photopic Vision = iota

	// Scotopic is the CIE 1951 V'(lambda) for rod vision, 380-780 nm,
	// peaking at 507 nm. It is the one a dark sky calls for.
	Scotopic
)

// String implements [fmt.Stringer].
func (v Vision) String() string {
	switch v {
	case Photopic:
		return "photopic"
	case Scotopic:
		return "scotopic"
	default:
		return "Vision(unknown)"
	}
}

// Luminous efficacy at each function's own peak, in lumens per watt.
//
// These convert a weighted radiance into a luminance and are definitional
// rather than measured: the lumen is defined so that 683 lm/W holds at
// 540 THz for photopic vision, and the scotopic maximum follows from the
// same definition applied to V'(lambda).
const (
	PhotopicEfficacy = 683.0
	ScotopicEfficacy = 1700.0
)

// Efficacy is the luminous efficacy that goes with this function.
func (v Vision) Efficacy() float64 {
	if v == Scotopic {
		return ScotopicEfficacy
	}

	return PhotopicEfficacy
}

// file is the CVRL table each function is published in.
func (v Vision) file() (string, error) {
	switch v {
	case Photopic:
		// The "e" is energy-based, which is what a radiance must be weighted
		// by; CVRL also publishes a quantal form that would be wrong here.
		return "vl1924e_1.csv", nil
	case Scotopic:
		return "scvle_1.csv", nil
	default:
		return "", fmt.Errorf("%w: unknown vision %d", ErrCurve, int(v))
	}
}

// Open fetches a luminous efficiency function and returns it as a passband.
//
// A [magnitude.Passband] because that is exactly what it is — a named
// spectral response curve — and because every projection in this module
// already knows how to integrate one. It carries no zero point: a luminance
// is not a magnitude and there is no flux standard to refer it to.
//
// The detector convention is [magnitude.EnergyIntegrating], which is not a
// choice. V(lambda) weights radiant power, so integrating it as though it
// counted photons would tilt the result across the band by a factor of
// lambda and leave a luminance that is smooth, positive and wrong.
//
// It is a download, so it is gated: grant [remote.EnableDownloads] for
// [remote.CVRLLuminosity] first. Both curves together are about 64 KB, and
// they never change — CIE 1924 and 1951 are fixed tabulations.
func Open(ctx context.Context, v Vision) (magnitude.Passband, error) {
	name, err := v.file()
	if err != nil {
		return magnitude.Passband{}, err
	}

	bucket, key, err := remote.GetFile(ctx, remote.CVRLLuminosity, name)
	if err != nil {
		return magnitude.Passband{}, fmt.Errorf("%w: fetch %s: %w", ErrCurve, name, err)
	}

	r, err := bucket.NewReader(ctx, key, nil)
	if err != nil {
		return magnitude.Passband{}, fmt.Errorf("%w: open %s: %w", ErrCurve, key, err)
	}

	defer func() { _ = r.Close() }()

	band, err := Parse(r)
	if err != nil {
		return magnitude.Passband{}, err
	}

	band.Name = "CIE " + v.String()
	band.Reference = "CVRL, UCL Institute of Ophthalmology (" + name + ")"

	return band, nil
}

// Parse reads CVRL's two-column "wavelength, value" CSV.
//
// Exported because it is the half of this package that needs no network, and
// because a caller holding a CIE tabulation from elsewhere — the standards
// themselves are not free — should be able to use it without this package
// fetching anything.
//
// Rows that do not parse are skipped rather than fatal. CVRL's files carry
// blank trailing lines, and some of its tables use an empty field where a
// function is undefined beyond its range; neither is a corrupt file, and
// refusing the whole curve over one would be the wrong trade.
func Parse(r io.Reader) (magnitude.Passband, error) {
	var (
		lambda   []unit.WavelengthNM
		response []float64
		prev     float64
	)

	scan := bufio.NewScanner(r)
	for scan.Scan() {
		nm, value, ok := parseRow(scan.Text())
		if !ok {
			continue
		}

		// Strictly increasing, because every consumer interpolates and a
		// repeated or reversed sample makes that silently wrong rather than
		// visibly broken.
		if len(lambda) > 0 && nm <= prev {
			return magnitude.Passband{}, fmt.Errorf("%w: wavelength %g follows %g",
				ErrCurve, nm, prev)
		}

		lambda = append(lambda, unit.WavelengthNM(nm))
		response = append(response, value)
		prev = nm
	}

	if err := scan.Err(); err != nil {
		return magnitude.Passband{}, fmt.Errorf("%w: %w", ErrCurve, err)
	}

	if len(lambda) < 2 {
		return magnitude.Passband{}, fmt.Errorf("%w: %d usable rows", ErrCurve, len(lambda))
	}

	return magnitude.Passband{
		WavelengthNM: lambda,
		Response:     response,
		Detector:     magnitude.EnergyIntegrating,
	}, nil
}

// parseRow reads one "wavelength, value" line.
func parseRow(line string) (nm, value float64, ok bool) {
	field := strings.SplitN(strings.TrimSpace(line), ",", 2)
	if len(field) != 2 {
		return 0, 0, false
	}

	nm, err := strconv.ParseFloat(strings.TrimSpace(field[0]), 64)
	if err != nil {
		return 0, 0, false
	}

	value, err = strconv.ParseFloat(strings.TrimSpace(field[1]), 64)
	if err != nil {
		return 0, 0, false
	}

	// A negative response is not a measurement of anything.
	if value < 0 {
		return 0, 0, false
	}

	return nm, value, true
}
