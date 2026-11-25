package fits

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/arrow/tensor"
)

// Section returns the pixels of the box [lower, upper) of the image: on axis
// i, the indices from lower[i] up to but not including upper[i], counted from
// zero in the order of Axes, slowest-varying first. The tensor has the box's
// shape, and the element type and stored values Tensor has for the same
// pixels; BSCALE, BZERO and BLANK apply to it as they apply to Tensor, through
// [ImageHDU.PhysicalValue].
//
// An image read by [ReadAt] or [OpenMmap] reads only the bytes the box covers,
// one read per contiguous run of it, so a box spanning whole rows is fewer and
// longer reads, and it holds only the box in memory, allocated from Arrow's
// default allocator. An image whose pixels are in Tensor, which must then be
// row-major, is cut from it.
//
// A box whose axes do not match the image's in number, or whose extent on any
// axis is empty, reversed or outside the image, is [ErrInvalidSection], and so
// is an image that holds no pixels, or a Tensor describing more pixels than
// its buffer holds. A source too short for the box is an error wrapping
// [io.ErrUnexpectedEOF].
//
// Any number of goroutines may take sections of one image at once, as
// [io.ReaderAt] allows its callers to read in parallel.
func (img *ImageHDU) Section(lower, upper []int64) (tensor.Interface, error) {
	src, base, dims, format, err := img.pixels()
	if err != nil {
		return nil, err
	}

	shape, n, err := sectionShape(dims, lower, upper)
	if err != nil {
		return nil, err
	}

	// No larger than the image, whose size pixels has checked fits.
	size := n * format.bytes

	// Only on a platform whose int is narrower than 64 bits.
	if size > math.MaxInt {
		return nil, fmt.Errorf("%w: %d bytes cannot be held", ErrInvalidSection, size)
	}

	buf := memory.NewResizableBuffer(memory.DefaultAllocator)
	buf.Resize(int(size))

	if err := readRuns(src, base, dims, lower, upper, format.bytes, buf.Bytes()); err != nil {
		buf.Release()

		return nil, err
	}

	// A lazily read image's bytes are as the file stores them; a Tensor's are
	// already decoded.
	if img.src != nil {
		format.decode(buf.Bytes())
	}

	data := array.NewData(format.dt, int(n), []*memory.Buffer{nil, buf}, nil, 0, 0)
	buf.Release()

	defer data.Release()

	return tensor.New(data, shape, nil, nil), nil
}

// openImage returns the image HDU header describes, its pixels left in r
// from off for [ImageHDU.Section] to read.
func openImage(h *Header, r io.ReaderAt, off int64) (*ImageHDU, error) {
	hdu, totalPixels, err := newImageHDU(h)
	if err != nil {
		return nil, err
	}

	if totalPixels == 0 {
		return hdu, nil
	}

	format, err := imageFormat(hdu.Bitpix, hdu.BZero)
	if err != nil {
		return nil, err
	}

	hdu.src, hdu.srcOffset, hdu.format = r, off, format

	return hdu, nil
}

// pixels returns where img's pixels are: the source a lazily read image keeps
// them in, as the file stores them, or Tensor's buffer, decoded. dims are the
// extents they are stored in, row-major. A lazily read image's size was
// checked at open; a tensor's shape is checked here against the buffer it
// describes, since a caller may have built it.
func (img *ImageHDU) pixels() (src io.ReaderAt, base int64, dims []int64, format pixelFormat, err error) {
	if img.src != nil {
		return img.src, img.srcOffset, img.Axes, img.format, nil
	}

	if img.Tensor == nil {
		return nil, 0, nil, pixelFormat{}, fmt.Errorf("%w: the image holds no pixels", ErrInvalidSection)
	}

	if !img.Tensor.IsRowMajor() {
		return nil, 0, nil, pixelFormat{}, fmt.Errorf("%w: the image's tensor is not row-major", ErrInvalidSection)
	}

	dt := img.Tensor.DataType()
	buffers := img.Tensor.Data().Buffers()

	// Every Arrow tensor holds a fixed-width type of whole bytes; the check
	// is the type assertion's.
	fixed, ok := dt.(arrow.FixedWidthDataType)
	if !ok || fixed.BitWidth()%8 != 0 || len(buffers) < 2 || buffers[1] == nil {
		return nil, 0, nil, pixelFormat{}, fmt.Errorf("%w: the image's tensor of %s holds no data buffer", ErrInvalidSection, dt)
	}

	width := int64(fixed.BitWidth() / 8)
	base = int64(img.Tensor.Data().Offset()) * width
	held := int64(len(buffers[1].Bytes())) - base

	described := width
	for _, d := range img.Tensor.Shape() {
		if described, err = mulSize(described, d); err != nil {
			return nil, 0, nil, pixelFormat{}, fmt.Errorf("%w: the image's tensor: %w", ErrInvalidSection, err)
		}
	}

	if described > held {
		return nil, 0, nil, pixelFormat{}, fmt.Errorf("%w: the image's tensor describes %d bytes and holds %d",
			ErrInvalidSection, described, held)
	}

	return bytes.NewReader(buffers[1].Bytes()), base, img.Tensor.Shape(), pixelFormat{dt: dt, bytes: width}, nil
}

// sectionShape checks the box [lower, upper) against an image of extents
// dims, and returns its shape and how many pixels it holds.
func sectionShape(dims, lower, upper []int64) (shape []int64, n int64, err error) {
	if len(dims) == 0 {
		return nil, 0, fmt.Errorf("%w: the image has no axes", ErrInvalidSection)
	}

	if len(lower) != len(dims) || len(upper) != len(dims) {
		return nil, 0, fmt.Errorf("%w: a box of %d and %d axes for an image of %d",
			ErrInvalidSection, len(lower), len(upper), len(dims))
	}

	shape = make([]int64, len(dims))
	n = 1

	for i, d := range dims {
		if lower[i] < 0 || upper[i] > d || lower[i] >= upper[i] {
			return nil, 0, fmt.Errorf("%w: axis %d: [%d, %d) is not a range inside [0, %d)",
				ErrInvalidSection, i, lower[i], upper[i], d)
		}

		shape[i] = upper[i] - lower[i]
		n *= shape[i] // at most the image's own pixel count, which fits
	}

	return shape, n, nil
}

// readRuns reads the box [lower, upper) of an image of extents dims, stored
// row-major from base in src at width bytes a pixel, into dst, which holds
// the box exactly.
//
// Each read is one contiguous run of the box: its extent on axis k, the
// fastest axis it does not span whole, times every faster axis, which it does.
// A box of whole rows is then one read per plane rather than one per row, and
// the whole image is one read.
func readRuns(src io.ReaderAt, base int64, dims, lower, upper []int64, width int64, dst []byte) error {
	last := len(dims) - 1

	k := last
	for k > 0 && lower[k] == 0 && upper[k] == dims[k] {
		k--
	}

	run := upper[k] - lower[k]
	for _, d := range dims[k+1:] {
		run *= d
	}

	runBytes := run * width

	stride := make([]int64, len(dims))
	stride[last] = 1

	for i := last - 1; i >= 0; i-- {
		stride[i] = stride[i+1] * dims[i+1]
	}

	// idx counts through the axes slower than k, each over its extent in the
	// box; every combination is the start of one run.
	idx := slices.Clone(lower[:k])

	for pos := int64(0); ; pos += runBytes {
		start := lower[k] * stride[k]
		for i, v := range idx {
			start += v * stride[i]
		}

		if err := readAtFull(src, dst[pos:pos+runBytes], base+start*width); err != nil {
			return err
		}

		i := k - 1
		for ; i >= 0; i-- {
			idx[i]++
			if idx[i] < upper[i] {
				break
			}

			idx[i] = lower[i]
		}

		if i < 0 {
			return nil
		}
	}
}

// readAtFull fills p from src at off. A ReaderAt may report io.EOF with a full
// read that ends the source, which is not a short read.
func readAtFull(src io.ReaderAt, p []byte, off int64) error {
	n, err := src.ReadAt(p, off)
	if n == len(p) {
		return nil
	}

	if err == nil || errors.Is(err, io.EOF) {
		return fmt.Errorf("fits: read section: %w: %d of %d bytes at offset %d", io.ErrUnexpectedEOF, n, len(p), off)
	}

	return fmt.Errorf("fits: read section: %w", err)
}
