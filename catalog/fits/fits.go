package fits

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/fits"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

// Sentinel errors for catalog/fits.
var (
	ErrNoBintable     = errors.New("catalog/fits: no binary table found")
	ErrMissingColumns = errors.New("catalog/fits: missing required mapping columns in bintable")
)

// Provider loads targets from a FITS binary table file.
type Provider struct {
	name    string
	targets []resolve.Target
}

// New encapsulates the entire process of opening a FITS file off disk,
// auto-detecting the first Binary Table HDU, and building a fully loaded resolve.
// It assumes the table uses standard simplistic column names ("ID", "NAME", "RA", "DEC").
func New(filePath string) (*Provider, error) {
	f, err := fits.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("catalog/fits: failed to open file: %w", err)
	}

	var bintable *fits.BintableHDU

	for _, hdu := range f.HDUs {
		if bt, ok := hdu.(*fits.BintableHDU); ok {
			bintable = bt
			break
		}
	}

	if bintable == nil {
		return nil, fmt.Errorf("%w: %s", ErrNoBintable, filePath)
	}

	ids, errID := bintable.GetStringColumn("ID")
	names, errName := bintable.GetStringColumn("NAME")
	ras, errRa := bintable.GetFloatColumn("RA")
	decs, errDec := bintable.GetFloatColumn("DEC")

	if errID != nil || errName != nil || errRa != nil || errDec != nil {
		return nil, ErrMissingColumns
	}

	// Every column comes back with NumRows entries, so these agree; taking the
	// shortest costs nothing and means a change in the reader cannot turn into
	// an out-of-range panic on a user-supplied file.
	rows := min(min(len(ids), len(names)), min(len(ras), len(decs)))
	targets := make([]resolve.Target, 0, rows)

	for i := range rows {
		target := resolve.Target{
			ID:      ids[i],
			Name:    names[i],
			Kind:    resolve.KindOther,
			Catalog: "FITS",
		}

		if usableCoord(ras[i], decs[i]) {
			target.Coord = coord.NewICRS(angle.Deg(ras[i]), angle.Deg(decs[i]))
			target.HasCoord = true
		}

		targets = append(targets, target)
	}

	catalogName := filepath.Base(filePath)

	return &Provider{
		name:    catalogName,
		targets: targets,
	}, nil
}

// usableCoord reports whether a row's right ascension and declination are a
// position rather than an artefact of reading one that was not there.
//
// GetFloatColumn writes the zero value for a null entry, so a row with no
// position does not arrive as an error or a NaN — it arrives as 0.0, and
// setting HasCoord on it puts a target in Cetus and points a telescope at it.
// This is the same failure catalog/mast was fixed for, and the one
// catalog.trustworthyCoord describes as having been observed in more than one
// provider; the fix belongs at the source, which is here.
//
// The bounds also catch the two substitutions a hand-built FITS table
// actually makes: a declination outside a right angle is not a declination,
// and a right ascension outside a full turn is one written in hours or in
// radians. Neither is repairable by guessing, so the row keeps its name and
// loses its claim to a position.
func usableCoord(ra, dec float64) bool {
	if math.IsNaN(ra) || math.IsNaN(dec) || math.IsInf(ra, 0) || math.IsInf(dec, 0) {
		return false
	}

	if dec < -90 || dec > 90 {
		return false
	}

	// Both conventions for right ascension are accepted, 0 to 360 and -180 to
	// 180; NewICRS wraps either.
	if ra < -360 || ra > 360 {
		return false
	}

	// Exactly zero in both is what a pair of null columns reads as. A genuine
	// target at that point would have to sit on the equator at the equinox to
	// the last bit of a float64, so reading it as absent is right far more
	// often than it is wrong.
	return ra != 0 || dec != 0
}

// Name returns the provider's literal identifier.
func (p *Provider) Name() string {
	return p.name
}

// Resolve attempts a precise match of a FITS target natively scanning ID or Name.
func (p *Provider) Resolve(query string) (resolve.Target, bool) {
	q := resolve.Normalize(query)
	for _, t := range p.targets {
		if resolve.Normalize(t.ID) == q || resolve.Normalize(t.Name) == q {
			return t, true
		}
	}

	return resolve.Target{}, false
}

// Search attempts substring matching, returning all intersecting records.
func (p *Provider) Search(query string) []resolve.Target {
	q := resolve.Normalize(query)

	var matches []resolve.Target
	if q == "" {
		return matches
	}

	for _, t := range p.targets {
		if strings.Contains(resolve.Normalize(t.ID), q) || strings.Contains(resolve.Normalize(t.Name), q) {
			matches = append(matches, t)
		}
	}

	return matches
}
