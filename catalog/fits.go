package catalog

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/fits"
)

type FITSProvider struct {
	name    string
	targets []Target
}

// NewFITSProvider encapsulates the entire process of opening a FITS file off disk,
// auto-detecting the first Binary Table HDU, and building a fully loaded Provider.
// It assumes the table uses standard simplistic column names ("ID", "NAME", "RA", "DEC").
func NewFITSProvider(filePath string) (*FITSProvider, error) {
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
		return nil, fmt.Errorf("catalog/fits: no binary table found in %s", filePath)
	}

	ids, errId := bintable.GetStringColumn("ID")
	names, errName := bintable.GetStringColumn("NAME")
	ras, errRa := bintable.GetFloatColumn("RA")
	decs, errDec := bintable.GetFloatColumn("DEC")

	if errId != nil || errName != nil || errRa != nil || errDec != nil {
		return nil, fmt.Errorf("catalog/fits: missing required mapping columns in bintable")
	}

	rows := len(ids)
	targets := make([]Target, 0, rows)

	for i := 0; i < rows; i++ {
		targets = append(targets, Target{
			ID:      ids[i],
			Name:    names[i],
			Kind:    KindOther,
			Coord:   coord.NewICRS(angle.Deg(ras[i]), angle.Deg(decs[i])),
			Catalog: "FITS",
		})
	}

	catalogName := filepath.Base(filePath)
	return &FITSProvider{
		name:    catalogName,
		targets: targets,
	}, nil
}

// Name returns the provider's literal identifier.
func (p *FITSProvider) Name() string {
	return p.name
}

// Resolve attempts a precise match of a FITS target natively scanning ID or Name.
func (p *FITSProvider) Resolve(query string) (Target, bool) {
	q := Normalize(query)
	for _, t := range p.targets {
		if Normalize(t.ID) == q || Normalize(t.Name) == q {
			return t, true
		}
	}
	return Target{}, false
}

// Search attempts substring matching, returning all intersecting records.
func (p *FITSProvider) Search(query string) []Target {
	q := Normalize(query)
	var matches []Target
	if q == "" {
		return matches
	}
	for _, t := range p.targets {
		if strings.Contains(Normalize(t.ID), q) || strings.Contains(Normalize(t.Name), q) {
			matches = append(matches, t)
		}
	}
	return matches
}


