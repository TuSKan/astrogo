package crosssection

import (
	"context"
	"errors"
	"fmt"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/remote"
)

// OzoneTemperatureK is the temperature of the shipped ozone cross section.
//
// Ozone peaks near 22 km, where the stratosphere runs roughly 220 to 230 K,
// so a laboratory table at 293 K describes the wrong gas. Total-column
// retrieval uses an effective temperature near 226 K; Serdyuchenko et al.
// measured at 10 K steps and 223 K is the nearest of them. Interpolating a
// table into existence to recover three kelvin would be inventing data for
// nothing.
//
// One assumption underlies using a single temperature at all: the Chappuis
// band, which is what matters in the visible, is far less temperature
// sensitive than the ultraviolet Huggins band. Part 2 of the reference is
// where that should be checked; if it does not hold, the cross section has to
// be weighted per scene against the atmosphere's own vertical profile rather
// than fixed here.
const OzoneTemperatureK = 223

// Ozone fetches the ozone absorption cross section.
//
// The download is consent-gated like every other bulk fetch: call
// [remote.EnableDownloads] with [remote.MPIMainzCrossSections] first, or this
// fails with [remote.ErrDownloadDenied]. The file is about 1.9 MB and covers
// 213 to 1100 nm, which spans the optical grid with no extrapolation at
// either end.
//
// Ozone is the absorber this package can represent honestly. Its Chappuis
// band is a broad continuum across the visible, which a tabulated cross
// section on a nanometre grid reproduces exactly. O2 and H2O absorb in narrow
// lines instead, and a band-averaged cross section is systematically wrong for
// them — see [atmosphere.CrossSection] and docs/skybrightness.md section 16.
func Ozone(ctx context.Context) (atmosphere.CrossSection, error) {
	bucket, key, err := remote.GetFile(ctx, remote.MPIMainzCrossSections, remote.OzoneSerdyuchenko223K)
	if err != nil {
		return atmosphere.CrossSection{}, fmt.Errorf("crosssection: fetch ozone: %w", err)
	}

	r, err := bucket.NewReader(ctx, key, nil)
	if err != nil {
		return atmosphere.CrossSection{}, fmt.Errorf("crosssection: open %s: %w", key, err)
	}
	defer func() { _ = r.Close() }()

	// The atlas publishes this file as two columns, wavelength in nanometres
	// and cross section in cm^2 per molecule, with no header. The unit is
	// passed rather than sniffed because the atlas also serves wavenumber and
	// angstrom files that a sniffer would confuse for one another.
	xs, err := Parse(r, "O3", Nanometre)
	if err != nil {
		return atmosphere.CrossSection{}, err
	}

	return xs, nil
}

// ErrLineAbsorber is returned for a species whose absorption cannot be
// represented as a cross section on a wavelength grid.
//
// It exists so the gap is programmatic rather than editorial. Before it, the
// reason O2 and H2O were absent lived in a doc comment and in
// docs/skybrightness.md; a caller who wanted them got no absorption and no
// explanation, which is indistinguishable from a model that considered them
// and found them negligible.
var ErrLineAbsorber = errors.New(
	"crosssection: this species absorbs in lines and needs a band model, not a cross section")

// ErrNoSource is returned for a species this package has no data for at all,
// which is a different situation from one it cannot represent.
var ErrNoSource = errors.New("crosssection: no source for this species")

// Species returns the absorption cross section for a molecule.
//
// Only ozone is available, and the refusal for the others is deliberate rather
// than pending. O2 (688 and 762 nm) and H2O (720, 820 and 940 nm) absorb in
// dense narrow lines. Averaging their cross section onto a nanometre grid and
// applying Beer-Lambert overestimates absorption systematically, because
// exp(-tau) is convex: the mean of the transmission is not the transmission of
// the mean. The error is invisible — every number stays positive and
// plausible — which is why this returns [ErrLineAbsorber] rather than a value.
//
// Representing them correctly means tabulating band-averaged transmittance
// against airmass rather than a cross section against wavelength, which is a
// different shape of dataset and a different calculation. HITRAN supplies the
// line lists when that exists.
func Species(ctx context.Context, name string) (atmosphere.CrossSection, error) {
	switch name {
	case "O3":
		return Ozone(ctx)
	case "O2", "H2O":
		return atmosphere.CrossSection{}, fmt.Errorf("%w: %s", ErrLineAbsorber, name)
	default:
		return atmosphere.CrossSection{}, fmt.Errorf("%w: %q", ErrNoSource, name)
	}
}
