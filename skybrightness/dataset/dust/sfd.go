package dust

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/apache/arrow-go/v18/arrow/tensor"

	"github.com/TuSKan/astrogo/angle"

	"github.com/TuSKan/astrogo/fits"
	"github.com/TuSKan/astrogo/remote"
)

// ErrSFD reports that the all-sky map could not be read.
var ErrSFD = errors.New("dust: SFD map")

// SFD is the Schlegel, Finkbeiner & Davis (1998) 100 micron all-sky map, held
// in memory and looked up locally.
//
// # Why this exists alongside Fetch
//
// [Fetch] asks IRSA one sightline per request, which answers in a couple of
// seconds and is the right thing for a handful of directions. It is the wrong
// thing for a sky: an all-sky map at one degree is twenty thousand sightlines,
// so twenty thousand requests to a shared service and thirteen hours. Dust is
// indexed by galactic direction, so a service answering for many sites and
// times converges on querying the whole sky one pixel at a time.
//
// This is the same data — the same map IRSA serves from — read once and
// consulted locally in nanoseconds. [Fetch] stays, both as the zero-setup path
// for a few directions and as the reference this is validated against.
//
// # Which map, and why it cannot be a better one
//
// SFD specifically. The diffuse-galactic correlation is fitted against this
// map and the 0.8 MJy/sr the model subtracts is this map's own zero-point
// term, so substituting a modern thermal-dust product would leave the
// coefficients describing something they were not fitted to.
type SFD struct {
	// north and south are the two hemispheres, 4096 squared each, in MJy/sr.
	north, south *hemisphere
}

// hemisphere is one polar projection of the map.
type hemisphere struct {
	// values is row-major, NAXIS2 rows of NAXIS1 columns.
	values []float64

	width, height int

	// nsgp is +1 for the north polar projection and -1 for the south, and
	// scale is the number of pixels from b = 0 to the pole. Both are read from
	// the file rather than assumed, because they are what the projection is.
	nsgp  float64
	scale float64

	// crpix1, crpix2 are the 1-indexed pixel coordinates of the pole.
	crpix1, crpix2 float64
}

// Open reads the all-sky map, downloading it once if it is not cached.
//
// Two files of 64 MB, one per galactic hemisphere, so the download is gated by
// [github.com/TuSKan/astrogo/remote.EnableDownloads] like every other bulk
// fetch. Held in memory afterwards: 4096 squared float64 per hemisphere is
// 268 MB, which is the cost of not asking a shared service twenty thousand
// questions.
func Open(ctx context.Context) (*SFD, error) {
	north, err := openHemisphere(ctx, northFile)
	if err != nil {
		return nil, err
	}

	south, err := openHemisphere(ctx, southFile)
	if err != nil {
		return nil, err
	}

	if north.nsgp <= 0 || south.nsgp >= 0 {
		return nil, fmt.Errorf("%w: the two files are not opposite hemispheres, "+
			"LAM_NSGP is %g and %g", ErrSFD, north.nsgp, south.nsgp)
	}

	return &SFD{north: north, south: south}, nil
}

// The Dataverse file identifiers of the two hemispheres, under
// doi:10.7910/DVN/EWCNL5. Not filenames: Dataverse addresses a deposited file
// by identifier, and the header check in openHemisphere is what confirms the
// identifier still points at the file it is supposed to.
const (
	northFile = "2902710"
	southFile = "2902711"
)

// openHemisphere fetches and decodes one hemisphere.
func openHemisphere(ctx context.Context, name string) (*hemisphere, error) {
	bucket, key, err := remote.GetFile(ctx, remote.SFDDustMap, name)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrSFD, name, err)
	}

	r, err := bucket.NewReader(ctx, key, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrSFD, name, err)
	}

	defer func() { _ = r.Close() }()

	f, err := fits.Read(r)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrSFD, name, err)
	}

	if len(f.HDUs) == 0 {
		return nil, fmt.Errorf("%w: %s holds no HDU", ErrSFD, name)
	}

	img, ok := f.HDUs[0].(*fits.ImageHDU)
	if !ok {
		return nil, fmt.Errorf("%w: %s is not an image", ErrSFD, name)
	}

	return fromImage(img, name)
}

// fromImage turns a decoded image into a hemisphere, checking that it is the
// map this package expects rather than trusting the identifier.
func fromImage(img *fits.ImageHDU, name string) (*hemisphere, error) {
	h := img.Header()

	// The unit is the whole point: the correlation takes an intensity in
	// MJy/sr, and the same deposit holds a reddening map in magnitudes whose
	// values are plausible and wrong.
	if unit, _ := h.GetString("BUNIT"); strings.TrimSpace(unit) != "MJy/sr" {
		return nil, fmt.Errorf("%w: %s is in %q, want MJy/sr — this may be the reddening "+
			"map rather than the 100 micron one", ErrSFD, name, unit)
	}

	// The projection, read from the file. CTYPE names it; LAM_NSGP says which
	// hemisphere and LAM_SCAL how many pixels span a quadrant.
	if ctype, _ := h.GetString("CTYPE1"); strings.TrimSpace(ctype) != "GLON-ZEA" {
		return nil, fmt.Errorf("%w: %s has CTYPE1 %q, want the galactic zenithal "+
			"equal-area projection", ErrSFD, name, ctype)
	}

	nsgp, errN := h.GetFloat("LAM_NSGP")
	scale, errS := h.GetFloat("LAM_SCAL")
	crpix1, errX := h.GetFloat("CRPIX1")
	crpix2, errY := h.GetFloat("CRPIX2")

	if errN != nil || errS != nil || errX != nil || errY != nil || scale <= 0 {
		return nil, fmt.Errorf("%w: %s does not carry a usable projection", ErrSFD, name)
	}

	if len(img.Axes) != 2 {
		return nil, fmt.Errorf("%w: %s has %d axes, want 2", ErrSFD, name, len(img.Axes))
	}

	// From the header keywords rather than from Axes, which is C-contiguous
	// and so reports rows before columns — the reverse of NAXIS1, NAXIS2. Both
	// are 4096 here, so reading it the wrong way round is invisible until the
	// day it is given a rectangular image.
	naxis1, err1 := h.GetInt("NAXIS1")
	naxis2, err2 := h.GetInt("NAXIS2")

	if err1 != nil || err2 != nil {
		return nil, fmt.Errorf("%w: %s does not state its dimensions", ErrSFD, name)
	}

	width, height := naxis1, naxis2

	values, err := pixels(img)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrSFD, name, err)
	}

	if len(values) != width*height {
		return nil, fmt.Errorf("%w: %s holds %d pixels for a %d by %d image",
			ErrSFD, name, len(values), width, height)
	}

	return &hemisphere{
		values: values, width: width, height: height,
		nsgp: nsgp, scale: scale, crpix1: crpix1, crpix2: crpix2,
	}, nil
}

// pixels reads the image payload as physical values.
//
// The decoder maps a FITS image onto an Arrow tensor whose element type
// follows BITPIX, so the stored values arrive typed. Only the two floating
// forms are handled: SFD is BITPIX -32 and an integer form would need BSCALE
// and BZERO applied per pixel, which is a different enough path that guessing
// at it would be worse than saying so.
func pixels(img *fits.ImageHDU) ([]float64, error) {
	switch t := img.Tensor.(type) {
	case *tensor.Float32:
		raw := t.Float32Values()

		out := make([]float64, len(raw))
		for i, v := range raw {
			out[i] = img.PhysicalValue(float64(v))
		}

		return out, nil

	case *tensor.Float64:
		raw := t.Float64Values()

		out := make([]float64, len(raw))
		for i, v := range raw {
			out[i] = img.PhysicalValue(v)
		}

		return out, nil

	default:
		return nil, fmt.Errorf("%w: BITPIX %d is not a floating image", ErrSFD, img.Bitpix)
	}
}

// IntensityAt returns the 100 micron intensity toward a galactic direction, in
// MJy sr^-1, satisfying [github.com/TuSKan/astrogo/skybrightness.DustMap].
func (s *SFD) IntensityAt(l, b angle.Angle) (float64, error) {
	if s == nil || s.north == nil || s.south == nil {
		return 0, fmt.Errorf("%w: not opened", ErrSFD)
	}

	// Each file covers its own hemisphere and overlaps the other a little past
	// the equator; taking the one the direction belongs to keeps every lookup
	// well inside its own projection rather than out at the edge where the
	// pixels are largest and the overlap is least reliable.
	h := s.south
	if b.Radians() >= 0 {
		h = s.north
	}

	return h.at(l, b)
}

// at samples one hemisphere, interpolating between pixels.
//
// The projection, from the file's own header:
//
//	X = sqrt(1 - NSGP*sin(b)) * cos(l) * SCALE
//	Y = -NSGP * sqrt(1 - NSGP*sin(b)) * sin(l) * SCALE
//
// which is the zenithal equal-area projection about the galactic pole. Equal
// area is what makes it the right one to hold this in: a pixel covers the same
// solid angle everywhere, so a mean over pixels is a mean over sky.
func (h *hemisphere) at(l, b angle.Angle) (float64, error) {
	x, y := h.pixelOf(l, b)

	return h.bilinear(x, y)
}

// pixelOf maps a galactic direction to a zero-indexed fractional pixel.
//
// The projection, transcribed from the file's own header, where it appears as
// the comment on CTYPE1 and CTYPE2:
//
//	X = sqrt(1 - NSGP*sin(b)) * cos(l) * SCALE
//	Y = -NSGP * sqrt(1 - NSGP*sin(b)) * sin(l) * SCALE
//
// Two things worth being explicit about. The radial coordinate goes as
// sqrt(1 - sin b) rather than as the zenith angle, which is what makes this
// equal-area: a pixel covers the same solid angle at the pole as at the
// equator, so an average over pixels is an average over sky. And FITS counts
// pixels from one while Go counts from zero, so CRPIX carries a minus one
// here; the half-pixel in CRPIX = 2048.50 is the file's own, saying the pole
// falls between pixels rather than on one.
func (h *hemisphere) pixelOf(l, b angle.Angle) (x, y float64) {
	// Only reachable a hair past the pole through rounding.
	radial := math.Max(0, 1-h.nsgp*math.Sin(b.Radians()))

	r := math.Sqrt(radial) * h.scale
	sinL, cosL := math.Sincos(l.Radians())

	return r*cosL + h.crpix1 - 1, -h.nsgp*r*sinL + h.crpix2 - 1
}

// bilinear samples the grid at a fractional pixel position.
//
// Interpolated rather than nearest-neighbour because the map is consulted at
// arbitrary directions and its pixels are 2.37 arcminutes: nearest-neighbour
// would make the intensity a step function of direction, and a component that
// integrates over the sky would then see edges that are the sampling's rather
// than the dust's.
func (h *hemisphere) bilinear(x, y float64) (float64, error) {
	if math.IsNaN(x) || math.IsNaN(y) {
		return 0, fmt.Errorf("%w: direction maps to (%g, %g)", ErrSFD, x, y)
	}

	x = math.Min(math.Max(x, 0), float64(h.width-1))
	y = math.Min(math.Max(y, 0), float64(h.height-1))

	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	x1, y1 := min(x0+1, h.width-1), min(y0+1, h.height-1)

	fx, fy := x-float64(x0), y-float64(y0)

	v00 := h.values[y0*h.width+x0]
	v10 := h.values[y0*h.width+x1]
	v01 := h.values[y1*h.width+x0]
	v11 := h.values[y1*h.width+x1]

	top := v00 + fx*(v10-v00)
	bottom := v01 + fx*(v11-v01)

	return top + fy*(bottom-top), nil
}
