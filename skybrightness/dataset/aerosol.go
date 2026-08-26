package dataset

import (
	"context"
	"fmt"
	"time"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/atmosphere/dataset/cams"
	"github.com/TuSKan/astrogo/coord"
)

// AerosolPreset is one of [atmosphere]'s OPAC-sourced aerosol constructors —
// [atmosphere.RuralAerosol], [atmosphere.UrbanAerosol],
// [atmosphere.DesertAerosol] or [atmosphere.MaritimeAerosol].
//
// A function value rather than an enum of this package's own, so that a
// constructor added there works here without being added twice, and so that
// the choice reads as the thing it is: which published aerosol type describes
// the air over this site.
type AerosolPreset func(heightM, aod550 float64) *atmosphere.Builder

// LiveAerosol builds an aerosol description using the optical depth actually
// measured over a site at an instant.
//
// # What this replaces
//
// The optical depth a caller would otherwise type. Every other aerosol number
// — single-scattering albedo, asymmetry, Angstrom exponent — is a property of
// the aerosol *type* and is legitimately constant, which is why OPAC can
// tabulate them. Optical depth is not: it is how much of that aerosol is
// overhead tonight, it moves by an order of magnitude at one site across a
// year, and a literal in the source is a guess wearing the same clothes as a
// measurement.
//
// So this is the opt-in tier the offline presets always pointed at:
//
//	air, err := dataset.LiveAerosol(ctx, site, when, atmosphere.RuralAerosol, 1538)
//	scene, err := sky.Scene(site, when, air)
//
// # Why it is not a default, and cannot be
//
// It downloads, which needs consent, and it reaches Copernicus over S3, which
// needs credentials the AWS chain resolves and a blank import of remote/s3
// the caller must make. None of that can be assumed of somebody who has just
// typed a preset name, so [atmosphere.CleanMountainAOD550] and its siblings
// remain the zero-setup path and this is the one you reach for deliberately.
//
// The value is a model analysis at roughly 40 km, not a measurement over the
// site — see [cams.AOD550] for what that means and when an AERONET station
// beats it.
func LiveAerosol(
	ctx context.Context,
	site *coord.Geodetic,
	when time.Time,
	preset AerosolPreset,
	scaleHeightM float64,
) (*atmosphere.Builder, error) {
	if site == nil {
		return nil, fmt.Errorf("%w: needs a site", ErrSpec)
	}

	if preset == nil {
		return nil, fmt.Errorf("%w: needs an aerosol preset, such as atmosphere.RuralAerosol",
			ErrSpec)
	}

	// The scale height is required rather than defaulted, because the OPAC
	// constructors do not set one and nothing here can source one per aerosol
	// type. A zero would build an atmosphere that looks complete and that
	// ArtificialSkyglow and CloudySkyglow both refuse, which is a worse
	// failure than being asked for the number.
	if scaleHeightM <= 0 {
		return nil, fmt.Errorf("%w: needs an aerosol scale height in metres; the models "+
			"reading it use one to two kilometres, and Kocifaj (2007) takes 1538 m",
			ErrSpec)
	}

	aod, err := cams.AOD550(ctx, site, when)
	if err != nil {
		return nil, fmt.Errorf("dataset: live aerosol: %w", err)
	}

	// The site's own elevation, which is what the OPAC constructors take:
	// they set surface conditions from the standard profile at that height.
	return preset(site.Height(), aod).AerosolScaleHeight(scaleHeightM), nil
}
