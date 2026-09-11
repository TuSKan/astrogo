package fits

import (
	"fmt"
	"strconv"
	"strings"
)

// An ASCII table (XTENSION = 'TABLE') stores its values as text, one
// fixed-width field per column, laid out by TBCOLn and formatted by TFORMn.
//
// It is the older of the two table extensions and BINTABLE superseded it, but
// archives are full of them and a library that cannot read one cannot read
// those archives. FITS 4.0 §7.2 is the reference.
//
// The formats are a small Fortran subset:
//
//	Iw      integer, right-justified in w columns
//	Fw.d    fixed-point, d digits after the decimal point
//	Ew.d    exponential, d digits of mantissa
//	Dw.d    exponential, double precision, the same layout as E
//	Aw      character, w columns
//
// A field of all blanks is undefined, which is the format's way of saying null
// — there is no TNULL here, and a blank is not zero.

// asciiForm is one parsed TFORMn of an ASCII table.
type asciiForm struct {
	code      byte
	width     int
	precision int
}

// parseASCIIForm reads a TFORMn such as "I10", "F12.5" or "A20".
func parseASCIIForm(tform string) (asciiForm, error) {
	t := strings.TrimSpace(tform)
	if t == "" {
		return asciiForm{}, fmt.Errorf("%w: empty TFORM", ErrBadTForm)
	}

	code := t[0]

	switch code {
	case 'I', 'F', 'E', 'D', 'A':
	default:
		return asciiForm{}, fmt.Errorf("%w: %q is not an ASCII table format", ErrBadTForm, tform)
	}

	rest := t[1:]

	widthText, precisionText, hasPrecision := strings.Cut(rest, ".")

	width, err := strconv.Atoi(strings.TrimSpace(widthText))
	if err != nil || width <= 0 {
		return asciiForm{}, fmt.Errorf("%w: width in %q", ErrBadTForm, tform)
	}

	form := asciiForm{code: code, width: width}

	if hasPrecision {
		p, err := strconv.Atoi(strings.TrimSpace(precisionText))
		if err != nil || p < 0 {
			return asciiForm{}, fmt.Errorf("%w: precision in %q", ErrBadTForm, tform)
		}

		form.precision = p
	}

	return form, nil
}

// String renders the form back as a TFORM value.
func (f asciiForm) String() string {
	if f.code == 'I' || f.code == 'A' {
		return string(f.code) + strconv.Itoa(f.width)
	}

	return string(f.code) + strconv.Itoa(f.width) + "." + strconv.Itoa(f.precision)
}

// parseCell decodes one field's text.
//
// blank reports a field of all spaces, which is how an ASCII table says the
// value is undefined. It is returned separately from the zero value because
// they are different things, and conflating them is how a missing measurement
// becomes a real one.
func (f asciiForm) parseCell(text string) (value float64, str string, blank bool, err error) {
	trimmed := strings.TrimSpace(text)

	if trimmed == "" {
		return 0, "", true, nil
	}

	if f.code == 'A' {
		return 0, strings.TrimRight(text, " "), false, nil
	}

	// FITS permits a D exponent where Go expects E, since Fortran wrote
	// double-precision literals that way.
	normalised := strings.Map(func(r rune) rune {
		if r == 'D' || r == 'd' {
			return 'E'
		}

		return r
	}, trimmed)

	v, err := strconv.ParseFloat(normalised, 64)
	if err != nil {
		return 0, "", false, fmt.Errorf("%w: %q is not a number", ErrBadTForm, trimmed)
	}

	return v, "", false, nil
}

// formatCell renders a value into its fixed-width field.
//
// A value too wide for its field is an error rather than a truncation: the
// columns are positional, so an overflowing field runs into its neighbour and
// every column after it is misread.
func (f asciiForm) formatCell(value float64, str string, null bool) (string, error) {
	if null {
		return strings.Repeat(" ", f.width), nil
	}

	var out string

	switch f.code {
	case 'A':
		out = str
	case 'I':
		out = strconv.FormatInt(int64(value), 10)
	case 'F':
		out = strconv.FormatFloat(value, 'f', f.precision, 64)
	case 'E', 'D':
		out = strconv.FormatFloat(value, 'E', f.precision, 64)
	default:
		return "", fmt.Errorf("%w: format %q", ErrBadTForm, string(f.code))
	}

	if len(out) > f.width {
		return "", fmt.Errorf("%w: %q needs %d columns, TFORM allows %d",
			ErrCardTooLong, out, len(out), f.width)
	}

	// Character fields are left-justified; everything else is right-justified,
	// which is what the format's Fortran ancestry prescribes and what a reader
	// aligning columns expects.
	if f.code == 'A' {
		return out + strings.Repeat(" ", f.width-len(out)), nil
	}

	return strings.Repeat(" ", f.width-len(out)) + out, nil
}
