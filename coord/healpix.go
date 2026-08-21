package coord

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
)

// Sentinel errors for HEALPix indexing.
var (
	// ErrHEALPixNside is returned for an nside that is not a power of two
	// in [1, 2^29]. The nested scheme's bit interleaving only works on
	// powers of two.
	ErrHEALPixNside = errors.New("coord: HEALPix nside must be a power of two")

	// ErrHEALPixPixel is returned for a pixel index outside [0, 12*nside^2).
	ErrHEALPixPixel = errors.New("coord: HEALPix pixel index out of range")
)

// HEALPix is the Hierarchical Equal Area isoLatitude Pixelation of the
// sphere at one resolution, in the NESTED ordering.
//
//   - Reference: Górski, K.M. et al. (2005), ApJ 622, 759.
//   - Pixels: 12*nside^2, every one of exactly equal solid angle 4*pi/npix.
//
// Equal area is the property that matters here. A sky map of integrated
// starlight stores a radiance per pixel, and radiance is per unit solid
// angle — so a tessellation whose pixels differ in area would need a
// per-pixel weight that is easy to forget and invisible when forgotten.
//
// NESTED rather than RING because it is what the sky-brightness maps this
// package consumes are published in, and because a nested index is its own
// hierarchy: dividing by four moves one level coarser.
type HEALPix struct {
	nside int64
	order int
}

// NewHEALPix returns the pixelation at the given nside, which must be a
// power of two.
func NewHEALPix(nside int64) (HEALPix, error) {
	if nside < 1 || nside > 1<<29 || nside&(nside-1) != 0 {
		return HEALPix{}, fmt.Errorf("%w: got %d", ErrHEALPixNside, nside)
	}

	order := 0
	for n := nside; n > 1; n >>= 1 {
		order++
	}

	return HEALPix{nside: nside, order: order}, nil
}

// Nside returns the resolution parameter.
func (h HEALPix) Nside() int64 { return h.nside }

// NumPixels returns 12*nside^2.
func (h HEALPix) NumPixels() int64 { return 12 * h.nside * h.nside }

// PixelArea returns the solid angle of one pixel, in steradians. Every
// pixel has this area exactly.
func (h HEALPix) PixelArea() float64 { return 4 * math.Pi / float64(h.NumPixels()) }

// PixelOf returns the nested pixel index containing a direction, given as
// longitude and latitude on whatever sphere the map is defined over —
// galactic, equatorial or horizontal. The pixelation itself is
// frame-agnostic; keeping the frames straight is the caller's job, and
// getting it wrong rotates the entire Milky Way across the sky.
//
// Latitude must lie in [-90, 90]. Outside that, and for a latitude or
// longitude that is not a number, the result is -1, which is not a pixel
// index; callers already test the returned index against the map's length,
// and a negative index fails that test.
//
// The guard is not pedantry. The index comes from sin(latitude), and sine
// folds: 120 degrees has the same sine as 60, so an out-of-domain latitude
// used to return the pixel at 60 degrees on the *same* longitude — while the
// direction "120 degrees of latitude" actually means 60 degrees at the
// longitude 180 away. The answer was a real direction, plausible in every
// downstream check, and not the one asked for.
func (h HEALPix) PixelOf(lon, lat angle.Angle) int64 {
	const outsideTheSphere = -1

	degrees := lat.Degrees()
	if math.IsNaN(degrees) || math.Abs(degrees) > 90 {
		return outsideTheSphere
	}

	if math.IsNaN(lon.Radians()) {
		return outsideTheSphere
	}

	z := math.Sin(lat.Radians())
	za := math.Abs(z)

	// Azimuth in units of 90 degrees, wrapped to [0, 4).
	tt := math.Mod(lon.Wrap2Pi().Radians()*(2/math.Pi), 4)
	if tt < 0 {
		tt += 4
	}

	var face, ix, iy int64

	if za <= 2.0/3.0 {
		face, ix, iy = h.equatorialPixel(tt, z)
	} else {
		face, ix, iy = h.polarPixel(tt, z, za)
	}

	return face*h.nside*h.nside + interleave(ix, iy)
}

// Center returns the longitude and latitude of a pixel's centre.
func (h HEALPix) Center(pixel int64) (lon, lat angle.Angle, err error) {
	if pixel < 0 || pixel >= h.NumPixels() {
		return 0, 0, fmt.Errorf("%w: %d not in [0, %d)", ErrHEALPixPixel, pixel, h.NumPixels())
	}

	nside2 := h.nside * h.nside
	face := pixel / nside2
	ix, iy := deinterleave(pixel % nside2)

	// Ring index counted from the north pole, and the position along it.
	jr := faceRingOffset[face]*h.nside - ix - iy - 1

	var nr, kshift int64

	var z float64

	switch {
	case jr < h.nside: // north polar cap
		nr = jr
		z = 1 - float64(nr)*float64(nr)/(3*float64(h.nside)*float64(h.nside))
		kshift = 0
	case jr > 3*h.nside: // south polar cap
		nr = 4*h.nside - jr
		z = float64(nr)*float64(nr)/(3*float64(h.nside)*float64(h.nside)) - 1
		kshift = 0
	default: // equatorial belt
		nr = h.nside
		z = float64(2*h.nside-jr) * 2 / (3 * float64(h.nside))
		kshift = (jr - h.nside) & 1
	}

	jp := (facePhiOffset[face]*nr + ix - iy + 1 + kshift) / 2
	if jp > 4*nr {
		jp -= 4 * nr
	}

	if jp < 1 {
		jp += 4 * nr
	}

	phi := (float64(jp) - float64(kshift+1)*0.5) * (math.Pi / 2 / float64(nr))

	return angle.Rad(phi).Wrap2Pi(), angle.Rad(math.Asin(math.Max(-1, math.Min(1, z)))), nil
}

// polarPixel handles the polar caps, |z| > 2/3, where the boundaries curve.
func (h HEALPix) polarPixel(tt, z, za float64) (face, ix, iy int64) {
	ntt := int64(tt)
	if ntt >= 4 {
		ntt = 3
	}

	tp := tt - float64(ntt)
	tmp := float64(h.nside) * math.Sqrt(3*(1-za))

	jp := int64(tp * tmp)
	jm := int64((1 - tp) * tmp)

	jp = min(jp, h.nside-1)
	jm = min(jm, h.nside-1)

	if z >= 0 {
		return ntt, h.nside - jm - 1, h.nside - jp - 1
	}

	return ntt + 8, jp, jm
}

// equatorialPixel handles the equatorial belt, |z| <= 2/3, where the
// pixel boundaries are straight lines in (phi, z).
func (h HEALPix) equatorialPixel(tt, z float64) (face, ix, iy int64) {
	temp1 := float64(h.nside) * (0.5 + tt)
	temp2 := float64(h.nside) * z * 0.75

	jp := int64(temp1 - temp2) // ascending edge line
	jm := int64(temp1 + temp2) // descending edge line

	ifp := jp >> h.order
	ifm := jm >> h.order

	switch {
	case ifp == ifm:
		face = (ifp & 3) + 4
	case ifp < ifm:
		face = ifp & 3
	default:
		face = (ifm & 3) + 8
	}

	return face, jm & (h.nside - 1), h.nside - (jp & (h.nside - 1)) - 1
}

// faceRingOffset and facePhiOffset place each of the twelve base faces in
// the ring/phi grid, per Górski et al. (2005) Fig. 4.
//
//nolint:gochecknoglobals // fixed geometry of the base tessellation
var (
	faceRingOffset = [12]int64{2, 2, 2, 2, 3, 3, 3, 3, 4, 4, 4, 4}
	facePhiOffset  = [12]int64{1, 3, 5, 7, 0, 2, 4, 6, 1, 3, 5, 7}
)

// interleave spreads the bits of x and y into a single Morton index, which
// is what makes a nested pixel number its own quadtree address.
//
// x and y are pixel coordinates within a face, so they lie in [0, nside) and
// the conversions to and from uint64 cannot overflow.
//
//nolint:gosec // bounded by nside, which NewHEALPix caps at 2^29
func interleave(x, y int64) int64 {
	return int64(spread(uint64(x)) | spread(uint64(y))<<1)
}

// spread inserts a zero bit between each bit of v.
func spread(v uint64) uint64 {
	u := v & 0xFFFFFFFF
	u = (u | u<<16) & 0x0000FFFF0000FFFF
	u = (u | u<<8) & 0x00FF00FF00FF00FF
	u = (u | u<<4) & 0x0F0F0F0F0F0F0F0F
	u = (u | u<<2) & 0x3333333333333333
	u = (u | u<<1) & 0x5555555555555555

	return u
}

// deinterleave is the inverse of interleave.
//
//nolint:gosec // m is a within-face pixel index, always non-negative
func deinterleave(m int64) (x, y int64) {
	return int64(compact(uint64(m))), int64(compact(uint64(m) >> 1))
}

// compact removes the interleaved zero bits.
func compact(v uint64) uint64 {
	u := v & 0x5555555555555555
	u = (u | u>>1) & 0x3333333333333333
	u = (u | u>>2) & 0x0F0F0F0F0F0F0F0F
	u = (u | u>>4) & 0x00FF00FF00FF00FF
	u = (u | u>>8) & 0x0000FFFF0000FFFF
	u = (u | u>>16) & 0x00000000FFFFFFFF

	return u
}
