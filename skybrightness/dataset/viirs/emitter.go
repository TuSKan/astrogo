package viirs

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"path"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/remote/file"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/unit"
)

// Region describes the source field to build around an observer.
//
// The raster is collapsed onto a ring of **azimuthally separated sources**,
// one per sector, because that is the structure Kocifaj & Bará (2019) Eq. 9
// requires: the total sky brightness is a sum over N sources on the horizon,
// each with its own line-of-sight radiance and azimuth. Nothing in the model
// stacks several sources at one azimuth.
//
// Within a sector the raster is walked outward in RadialSamples steps and the
// samples are combined into one equivalent source. Refining RadialSamples
// therefore improves the estimate of a fixed quantity rather than multiplying
// the number of sources — the emitter count is set by Sectors alone.
//
// # What this does not do
//
// Kocifaj & Bará say L_S "can be inferred from satellite radiance data",
// citing Elvidge et al. (2017). That citation is to the data — Elvidge et al.
// is an instrument and product description, and carries no conversion from a
// DNB pixel to a line-of-sight radiance.
//
// # What a published recipe does cover, and what it leaves
//
// Aubé et al. (2020), arXiv:2005.14160 §2.4, correct the DNB signal for
// atmospheric extinction and for subgrid obstacles — trees and buildings that
// block the low-angle light the satellite would otherwise see — as
// Ra_corrected = Ra * F_T * F_o, with F_T = 1/(Ta*Tm) and
//
//	theta_lim = arctan(d_o / h_o)
//	F_o = (1 - cos70) / [1 - f_o*cos(theta_lim) + (f_o - 1)*cos70]
//
// over obstacle height h_o, lamp-to-obstacle distance d_o and filling factor
// f_o. For Tenerife's 9 m, 4 m and 0.9 that is F_o about 4.6.
//
// It would apply cleanly here: [skybrightness.ArtificialSkyglow] is
// homogeneous in source strength to 2e-16, so a factor like this belongs on
// an emitter rather than on a result — better than that paper's own
// correction, which is applied to model output with one obstacle set for the
// whole domain and which its authors name as the likely source of their
// 10 per cent residual.
//
// What it does not give is the reason this package still has no absolute
// scale. F_o integrates over the 0 to 70 degree cone VIIRS samples, so it
// anchors how much light goes up and says nothing about the *shape* of the
// upward emission — and near-horizontal emission is what carries skyglow far,
// which is why Garstang's function carries a z0^4 term at all. Illumina takes
// that shape, and the source spectra, from its own inventory. So the recipe
// settles part of one of the four quantities named above, and needs obstacle
// parameters this package does not have besides.
//
// What this package does instead is stated plainly: it sums the upward
// radiances along each azimuth and places the result at the radiance-weighted
// mean distance. That preserves Eq. 9's one-source-per-azimuth structure and
// the relative weighting between azimuths, and it is not a published
// inference. Treat the absolute scale as uncalibrated and the directional
// structure as meaningful.
//
// The one thing Elvidge et al. does settle is the unit: the annual composites
// are average radiances in nW cm^-2 sr^-1, which is what SourceSpectrum
// converts from.
type Region struct {
	// Site is the observer, at the centre of the sampling.
	Site *coord.Geodetic

	// InnerM and OuterM bound the annulus sampled, in metres. Sources much
	// closer than a kilometre break the horizon-source geometry the
	// skyglow component assumes; sources far beyond a hundred kilometres
	// contribute little through the transmission term.
	InnerM, OuterM float64

	// Sectors is the number of azimuthal sources produced, and so the
	// number of emitters. It is the model's own discretisation.
	Sectors int

	// RadialSamples is how finely each azimuth is walked outward. It
	// refines the estimate within a sector and does not change the emitter
	// count.
	RadialSamples int

	// Spectrum is the assumed source spectral shape and the sensor response
	// the DNB radiance was measured through.
	Spectrum SourceSpectrum

	// Emission is the assumed upward emission function, shared by every
	// emitter. It is the single most consequential assumption here: the
	// same pixel with and without shielding is a different sky.
	Emission skybrightness.UpwardEmission

	// MinRadiance drops samples below this DNB radiance, in nW cm^-2 sr^-1.
	// The VIIRS noise floor is around 0.5 in dark areas, so a small
	// positive value avoids turning sensor noise into light sources.
	MinRadiance float64
}

// defaults for a Region left partly unset.
const (
	defaultInnerM        = 1_000.0
	defaultOuterM        = 100_000.0
	defaultSectors       = 36
	defaultRadialSamples = 40
)

// withDefaults returns a copy with unset fields filled in, and validates it.
func (r Region) withDefaults() (Region, error) {
	if r.Site == nil {
		return r, fmt.Errorf("%w: no site", ErrSampling)
	}

	if r.InnerM <= 0 {
		r.InnerM = defaultInnerM
	}

	if r.OuterM <= 0 {
		r.OuterM = defaultOuterM
	}

	if r.Sectors <= 0 {
		r.Sectors = defaultSectors
	}

	if r.RadialSamples <= 0 {
		r.RadialSamples = defaultRadialSamples
	}

	if r.OuterM <= r.InnerM {
		return r, fmt.Errorf("%w: outer radius %g is not beyond inner %g", ErrSampling, r.OuterM, r.InnerM)
	}

	return r, nil
}

// Emitters collapses the raster onto one [skybrightness.GroundEmitter] per
// azimuth sector.
//
// Each emitter carries the sector's summed DNB radiance converted to a
// spectral radiance through the region's assumed spectrum, sits at the
// radiance-weighted mean distance along that azimuth, and is flagged
// AssumedSourceSpectrum and AssumedEmissionFunction — because both are.
//
// Samples outside the raster's coverage or resolving only to no-data are
// skipped rather than treated as dark: missing data is not measured
// darkness, and conflating them would make an unmapped region look pristine.
// A sector with no usable samples produces no emitter at all.
func (r *Raster) Emitters(region Region) ([]skybrightness.GroundEmitter, error) {
	region, err := region.withDefaults()
	if err != nil {
		return nil, err
	}

	scale, err := region.Spectrum.scale()
	if err != nil {
		return nil, err
	}

	out := make([]skybrightness.GroundEmitter, 0, region.Sectors)

	step := (region.OuterM - region.InnerM) / float64(region.RadialSamples)
	sectorStep := 360.0 / float64(region.Sectors)

	for sector := range region.Sectors {
		bearing := angle.Deg((float64(sector) + 0.5) * sectorStep)

		total, weighted, err := r.walkAzimuth(region, bearing, step)
		if err != nil {
			return nil, err
		}

		if total <= 0 {
			continue
		}

		at, err := coord.Offset(region.Site, bearing, weighted/total)
		if err != nil {
			return nil, fmt.Errorf("viirs: sector %d: %w", sector, err)
		}

		out = append(out, emitterFor(at, total, scale, region))
	}

	return out, nil
}

// walkAzimuth sums the usable radiance along one bearing and accumulates the
// radiance-distance product, so the caller can place the equivalent source at
// the radiance-weighted mean distance.
func (r *Raster) walkAzimuth(region Region, bearing angle.Angle, step float64) (total, weighted float64, err error) {
	for i := range region.RadialSamples {
		distance := region.InnerM + (float64(i)+0.5)*step

		at, err := coord.Offset(region.Site, bearing, distance)
		if err != nil {
			return 0, 0, fmt.Errorf("viirs: sample at %.0f m: %w", distance, err)
		}

		radiance, err := r.RadianceAt(at.Lon().Degrees(), at.Lat().Degrees())
		if err != nil {
			continue // outside coverage, or no data: not darkness
		}

		if radiance < region.MinRadiance || radiance <= 0 {
			continue
		}

		total += radiance
		weighted += radiance * distance
	}

	return total, weighted, nil
}

// emitterFor builds one emitter from a bin's radiance.
func emitterFor(at *coord.Geodetic, radiance float64, scale []float64, region Region) skybrightness.GroundEmitter {
	spectral := make([]float64, len(scale))
	for i, s := range scale {
		spectral[i] = radiance * s
	}

	return &skybrightness.UniformEmitter{
		At:   at,
		Name: fmt.Sprintf("viirs %.1f nW/cm2/sr", radiance),
		WavelengthNM: append([]unit.WavelengthNM(nil),
			region.Spectrum.WavelengthNM...),
		Radiance: spectral,
		Emission: region.Emission,
		Flags: skybrightness.AssumedSourceSpectrum |
			skybrightness.AssumedEmissionFunction,
	}
}

// extractTIFF unpacks the single GeoTIFF entry from a downloaded archive
// into the same bucket, and returns its key. An already-extracted object is
// reused.
func extractTIFF(ctx context.Context, bucket *file.Bucket, archiveKey string, year int) (string, error) {
	tiffKey := entryName(year)

	if ok, err := bucket.Exists(ctx, tiffKey); err == nil && ok {
		return tiffKey, nil
	}

	at, err := file.NewReaderAt(ctx, bucket, archiveKey)
	if err != nil {
		return "", fmt.Errorf("viirs: open archive: %w", err)
	}
	defer closeQuietly(at)

	entry, closeEntry, err := openZIPEntry(at, at.Size(), tiffKey)
	if err != nil {
		return "", err
	}
	defer closeEntry()

	if err := file.Save(ctx, bucket, tiffKey, entry); err != nil {
		return "", fmt.Errorf("viirs: extract %s: %w", tiffKey, err)
	}

	return tiffKey, nil
}

// closeQuietly closes c, discarding an error that a caller in a failure path
// can do nothing about.
func closeQuietly(c io.Closer) {
	if c != nil {
		_ = c.Close()
	}
}

// openZIPEntry opens the named entry of a zip archive for streaming.
//
// The archive is read through an io.ReaderAt rather than buffered: these
// composites are hundreds of megabytes compressed, and archive/zip only needs
// the central directory plus the one entry's bytes.
func openZIPEntry(r io.ReaderAt, size int64, name string) (io.Reader, func(), error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, nil, fmt.Errorf("viirs: read archive: %w", err)
	}

	for _, f := range zr.File {
		// Compare on the base name: some years nest the entry in a folder.
		if path.Base(f.Name) != name {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return nil, nil, fmt.Errorf("viirs: open %s in archive: %w", name, err)
		}

		return rc, func() { closeQuietly(rc) }, nil
	}

	return nil, nil, fmt.Errorf("%w: archive has no entry %q", ErrArchive, name)
}
