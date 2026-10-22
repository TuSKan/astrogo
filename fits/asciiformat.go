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
// An undefined value is the column's TNULLn string, implicitly space filled to
// the field's width (§7.2.2). A blank integer field is 0, which §7.2.5 says
// outright; a blank real or character field, which it gives no value, reads
// as undefined. These comments used to say a blank was the format's null and
// that "there is no TNULL here", and the reader and writer were built on it:
// a declared null string sank the whole read, a numeric one became a real
// value, and a null this package wrote read as 0 in a reader keeping to the
// standard (#771).

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

// parseCell decodes one field's text by FITS 4.0 §7.2.5.
//
// null reports a field with no value: one matching the column's TNULLn
// (hasNull), compared without its padding, or a blank real or character field.
// It is returned separately from the zero value because they are different
// things, and conflating them is how a missing measurement becomes a real one.
// A blank integer field is not null but 0, as §7.2.5 says.
func (f asciiForm) parseCell(text, tnull string, hasNull bool) (value float64, str string, null bool, err error) {
	trimmed := strings.TrimSpace(text)

	if hasNull && trimmed == strings.TrimSpace(tnull) {
		return 0, "", true, nil
	}

	switch {
	case trimmed == "" && f.code == 'I':
		return 0, "", false, nil
	case trimmed == "":
		return 0, "", true, nil
	case f.code == 'A':
		return 0, strings.TrimRight(text, " "), false, nil
	}

	v, err := f.parseNumber(trimmed)
	if err != nil {
		return 0, "", false, fmt.Errorf("%w: %q is not a number", ErrBadTForm, trimmed)
	}

	return v, "", false, nil
}

// parseNumber reads a numeric field's text, already trimmed.
//
// The standard's form is a signed digit string with at most one decimal
// point, then optionally an exponent introduced by E, D or a bare sign:
// "1.5-3" is 1.5e-3. A real field written without a decimal point has one
// implied before its rightmost d digits, so "12345" in F8.2 is 123.45. That
// form is deprecated but still defined, and reading it as 12345 is a factor of
// a hundred no reader would notice.
//
// Text outside that form, such as the nan or inf some writers put in a real
// field, is read as strconv reads it, with a D exponent taken for E.
func (f asciiForm) parseNumber(s string) (float64, error) {
	mantissa, exponent, ok := splitFortranNumber(s)
	if !ok {
		v, err := strconv.ParseFloat(strings.NewReplacer("D", "E", "d", "e").Replace(s), 64)
		if err != nil {
			return 0, fmt.Errorf("parse %q: %w", s, err)
		}

		return v, nil
	}

	if f.code != 'I' && f.precision > 0 && !strings.Contains(mantissa, ".") {
		mantissa = impliedDecimal(mantissa, f.precision)
	}

	if exponent != "" {
		mantissa += "e" + exponent
	}

	v, err := strconv.ParseFloat(mantissa, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %q: %w", s, err)
	}

	return v, nil
}

// splitFortranNumber splits s into its mantissa, sign included, and its
// exponent as a signed integer, reporting false if s is not in §7.2.5's form.
func splitFortranNumber(s string) (mantissa, exponent string, ok bool) {
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}

	digits, points := 0, 0

	for ; i < len(s); i++ {
		if s[i] == '.' {
			points++

			continue
		}

		if s[i] < '0' || s[i] > '9' {
			break
		}

		digits++
	}

	if digits == 0 || points > 1 {
		return "", "", false
	}

	mantissa, rest := s[:i], s[i:]

	switch {
	case rest == "":
		return mantissa, "", true
	case strings.ContainsRune("EeDd", rune(rest[0])):
		rest = rest[1:]
	case rest[0] != '+' && rest[0] != '-':
		return "", "", false
	}

	j := 0
	if j < len(rest) && (rest[j] == '+' || rest[j] == '-') {
		j++
	}

	if j == len(rest) {
		return "", "", false
	}

	for _, c := range rest[j:] {
		if c < '0' || c > '9' {
			return "", "", false
		}
	}

	return mantissa, rest, true
}

// impliedDecimal places a decimal point before the rightmost d digits of a
// signed digit string, with leading zeros assumed where there are fewer.
func impliedDecimal(mantissa string, d int) string {
	sign, digits := "", mantissa
	if digits[0] == '+' || digits[0] == '-' {
		sign, digits = digits[:1], digits[1:]
	}

	if len(digits) < d {
		digits = strings.Repeat("0", d-len(digits)) + digits
	}

	return sign + digits[:len(digits)-d] + "." + digits[len(digits)-d:]
}

// formatCell renders a value into its fixed-width field.
//
// A value too wide for its field is an error rather than a truncation: the
// columns are positional, so an overflowing field runs into its neighbor and
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
