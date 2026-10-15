package fits_test

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/fits"
	"github.com/TuSKan/astrogo/time"
)

// tableFile is an empty primary HDU followed by a one-row table extension of
// type xtension, declaring tfields fields over row.
func tableFile(xtension, tfields string, row []byte, columns ...string) []byte {
	cards := append([]string{
		card("XTENSION", xtension), card("BITPIX", "8"), card("NAXIS", "2"),
		card("NAXIS1", strconv.Itoa(len(row))), card("NAXIS2", "1"),
		card("PCOUNT", "0"), card("GCOUNT", "1"), card("TFIELDS", tfields),
	}, columns...)

	return append(emptyPrimary(), hduBytes(row, cards...)...)
}

// TestTableFieldCountIsBounded: TFIELDS sizes every per-column slice before a
// byte of the table is read, so it is held to the standard's 0 … 999 first
// (FITS 4.0 §7.2.1, §7.3.1). -1 panicked, and 100000000000 asked for 8.8 TB,
// which in Go is a fatal error rather than a returned one (#796): against the
// code before the fix, that case ends the test binary.
func TestTableFieldCountIsBounded(t *testing.T) {
	t.Parallel()

	for _, xtension := range []string{"'BINTABLE'", "'TABLE   '"} {
		for _, tfields := range []string{"-1", "1000", "100000000000"} {
			t.Run(xtension+" TFIELDS = "+tfields, func(t *testing.T) {
				t.Parallel()

				raw := tableFile(xtension, tfields, make([]byte, 8), card("TFORM1", "'D'"), card("TBCOL1", "1"))

				if _, err := fits.Read(bytes.NewReader(raw)); err == nil || !strings.Contains(err.Error(), "TFIELDS") {
					t.Errorf("Read: err %v, want one naming TFIELDS", err)
				}
			})
		}
	}
}

// TestTableHoldsTheStandardsMostFields: 999 is the standard's maximum, not one
// past it, and a table of 999 fields reads in full, of either kind.
func TestTableHoldsTheStandardsMostFields(t *testing.T) {
	t.Parallel()

	const fields = 999

	row := bytes.Repeat([]byte{'7'}, fields)

	for _, c := range []struct {
		xtension string
		column   func(n string) []string
	}{
		{"'BINTABLE'", func(n string) []string { return []string{card("TFORM"+n, "'B'")} }},
		{"'TABLE   '", func(n string) []string { return []string{card("TFORM"+n, "'I1'"), card("TBCOL"+n, n)} }},
	} {
		var columns []string
		for i := 1; i <= fields; i++ {
			columns = append(columns, c.column(strconv.Itoa(i))...)
		}

		f, err := fits.Read(bytes.NewReader(tableFile(c.xtension, strconv.Itoa(fields), row, columns...)))
		if err != nil {
			t.Errorf("%s of %d fields: %v", c.xtension, fields, err)

			continue
		}

		var got int64

		switch hdu := f.HDUs[1].(type) {
		case *fits.BintableHDU:
			got = hdu.Batch.NumCols()
		case *fits.ASCIITableHDU:
			got = hdu.Batch.NumCols()
		default:
			t.Fatalf("%s read as %T", c.xtension, hdu)
		}

		if got != fields {
			t.Errorf("%s: %d columns, want %d", c.xtension, got, fields)
		}
	}
}

// TestExtractWCSBoundsNAXIS: ExtractWCS reads NAXIS itself, and a table's
// header reaches it with an NAXIS Read never looked at. 100000000000 sized
// its axis slices at 800 GB, a fatal error (#796); against the code before the
// fix, that case ends the test binary. 1000 is the first value past the
// standard's cap, and was accepted.
func TestExtractWCSBoundsNAXIS(t *testing.T) {
	t.Parallel()

	for _, naxis := range []string{"1000", "100000000000"} {
		_, err := fits.ExtractWCS(wcsHeader(t, "NAXIS = "+naxis, "CTYPE1 = 'RA---TAN'", "CTYPE2 = 'DEC--TAN'"))
		if err == nil || !strings.Contains(err.Error(), "exceeds 999") {
			t.Errorf("NAXIS = %s: err %v, want one saying it exceeds 999", naxis, err)
		}
	}

	if _, err := fits.ExtractWCS(wcsHeader(t, "NAXIS = 999")); err != nil {
		t.Errorf("NAXIS = 999, the standard's maximum: %v", err)
	}
}

// withSIPOrder is sipCards with both orders replaced.
func withSIPOrder(order string) []string {
	base := sipCards("E")
	cards := make([]string, 0, len(base))

	for _, c := range base {
		if k, _, _ := strings.Cut(c, "="); strings.HasSuffix(strings.TrimSpace(k), "_ORDER") {
			c = strings.TrimSpace(k) + " = " + order
		}

		cards = append(cards, c)
	}

	return cards
}

// TestSIPOrderDoesNotSetTheWork: a SIP polynomial is read from the coefficient
// cards the header holds. It used to be looked up for every term its order
// allowed, (order+1)(order+2)/2 keywords at about 140 ns each, so one card
// claiming an order of a billion held ExtractWCS for millennia (#796). The
// same coefficients under any order that admits them are the same WCS.
//
// The deadline turns that hang into a failure that names it, rather than a
// test binary killed by its timeout. Read from the cards, the work takes
// microseconds, so a minute cannot be a flake.
func TestSIPOrderDoesNotSetTheWork(t *testing.T) {
	t.Parallel()

	small, err := fits.ExtractWCS(wcsHeader(t, withSIPOrder("2")...))
	if err != nil {
		t.Fatalf("order 2: %v", err)
	}

	h := wcsHeader(t, withSIPOrder("1000000000")...)

	var (
		huge    *fits.WCS
		hugeErr error
		done    = make(chan struct{})
	)

	go func() {
		defer close(done)

		huge, hugeErr = fits.ExtractWCS(h)
	}()

	select {
	case <-done:
	case <-time.After(time.Minute):
		t.Fatal("ExtractWCS has not returned in a minute: the declared order is setting the work again")
	}

	if hugeErr != nil {
		t.Fatalf("order 1000000000: %v", hugeErr)
	}

	for _, pixel := range [][]float64{{1, 1}, {2048, 2048}, {1, 2048}} {
		a, err := small.PixelToWorld(pixel)
		if err != nil {
			t.Fatalf("order 2: PixelToWorld(%v): %v", pixel, err)
		}

		b, err := huge.PixelToWorld(pixel)
		if err != nil {
			t.Fatalf("order 1000000000: PixelToWorld(%v): %v", pixel, err)
		}

		if a[0] != b[0] || a[1] != b[1] {
			t.Errorf("PixelToWorld(%v): %v under order 2, %v under order 1000000000", pixel, a, b)
		}
	}
}

// TestSIPTransformIsReproducible: the same pixel through the same WCS gives
// the same world coordinate, every call. The SIP terms were summed in map
// order, which Go randomizes, so under this ordinary fourth-order distortion
// some pixels came back with more than one answer: 5 of these 1,089 over 40
// calls each, in the run that found it (#796).
func TestSIPTransformIsReproducible(t *testing.T) {
	t.Parallel()

	w := fits.NewWCS(2)

	for _, err := range []error{
		w.SetCTYPE([]string{"RA---TAN-SIP", "DEC--TAN-SIP"}),
		w.SetCRVAL([]float64{150, 2}),
		w.SetCRPIX([]float64{1024, 1024}),
		w.SetCDELT([]float64{-2.8e-4, 2.8e-4}),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}

	// Coefficients falling by 10^4 an order, as a real fit's do.
	a, b := map[[2]int]float64{}, map[[2]int]float64{}
	scale := map[int]float64{2: 1, 3: 1e-4, 4: 1e-8}

	for p := range 5 {
		for q := range 5 - p {
			if p+q < 2 {
				continue
			}

			a[[2]int{p, q}] = 1.37e-5 * scale[p+q] / float64(p+3*q+1)
			b[[2]int{p, q}] = -2.11e-6 * scale[p+q] * float64(2*p+q+1)
		}
	}

	w.SetSIP(a, b)

	for x := 1.0; x <= 2048; x += 63.7 {
		for y := 1.0; y <= 2048; y += 63.7 {
			first, err := w.PixelToWorld([]float64{x, y})
			if err != nil {
				t.Fatalf("PixelToWorld(%v, %v): %v", x, y, err)
			}

			for range 40 {
				again, err := w.PixelToWorld([]float64{x, y})
				if err != nil {
					t.Fatalf("PixelToWorld(%v, %v): %v", x, y, err)
				}

				if again[0] != first[0] || again[1] != first[1] {
					t.Fatalf("PixelToWorld(%v, %v) gave %v and then %v", x, y, first, again)
				}
			}
		}
	}
}
