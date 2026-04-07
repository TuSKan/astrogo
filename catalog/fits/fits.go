package fits

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/fits"

	"github.com/TuSKan/astrogo/catalog/provider"
)

type Provider struct {
	name    string
	targets []provider.Target
}

// New encapsulates the entire process of opening a FITS file off disk,
// auto-detecting the first Binary Table HDU, and building a fully loaded Provider.
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
	targets := make([]provider.Target, 0, rows)

	for i := 0; i < rows; i++ {
		targets = append(targets, provider.Target{
			ID:      ids[i],
			Name:    names[i],
			Kind:    provider.KindOther,
			Coord:   coord.NewICRS(angle.Deg(ras[i]), angle.Deg(decs[i])),
			Catalog: "FITS",
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
func (p *Provider) Resolve(query string) (provider.Target, bool) {
	q := provider.Normalize(query)
	for _, t := range p.targets {
		if provider.Normalize(t.ID) == q || provider.Normalize(t.Name) == q {
			return t, true
		}
	}
	return provider.Target{}, false
}

// Search attempts substring matching, returning all intersecting records.
func (p *Provider) Search(query string) []provider.Target {
	q := provider.Normalize(query)
	var matches []provider.Target
	if q == "" {
		return matches
	}
	for _, t := range p.targets {
		if strings.Contains(provider.Normalize(t.ID), q) || strings.Contains(provider.Normalize(t.Name), q) {
			matches = append(matches, t)
		}
	}
	return matches
}
