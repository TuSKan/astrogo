package fits

import (
	"errors"
	"fmt"
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

	ids, errId := bintable.GetStringColumn("ID")
	names, errName := bintable.GetStringColumn("NAME")
	ras, errRa := bintable.GetFloatColumn("RA")
	decs, errDec := bintable.GetFloatColumn("DEC")

	if errId != nil || errName != nil || errRa != nil || errDec != nil {
		return nil, ErrMissingColumns
	}

	rows := len(ids)
	targets := make([]resolve.Target, 0, rows)

	for i := range rows {
		targets = append(targets, resolve.Target{
			ID:       ids[i],
			Name:     names[i],
			Kind:     resolve.KindOther,
			Coord:    coord.NewICRS(angle.Deg(ras[i]), angle.Deg(decs[i])),
			HasCoord: true,
			Catalog:  "FITS",
		})
	}

	catalogName := filepath.Base(filePath)

	return &Provider{
		name:    catalogName,
		targets: targets,
	}, nil
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
