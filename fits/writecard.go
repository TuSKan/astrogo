package fits

import (
	"fmt"
	"strings"
)

// The fixed-format card layout, from FITS 4.0 §4.1.2. These are column
// positions in a 1-based standard rendered into a 0-based slice, which is
// where an off-by-one in a FITS writer traditionally lives, so they are named
// rather than spelled inline.
const (
	// keywordWidth is columns 1-8, where the keyword sits left-justified.
	keywordWidth = 8

	// valueIndicator occupies columns 9-10 of a value card.
	valueIndicator = "= "

	// fixedValueEnd is the column a fixed-format numeric or logical value
	// ends at, right-justified. The standard asks for this alignment and
	// readers in other languages have been known to rely on it.
	fixedValueEnd = 30

	// minStringValue is the minimum length of a quoted string's contents.
	// A shorter one is blank-padded, so that 'a' is written 'a       '.
	minStringValue = 8
)

// commentaryKeywords carry free text in columns 9-80 and take no value
// indicator. Writing "COMMENT = ..." would be a syntax error to every reader.
//
// The blank keyword is the third of them and is how a FITS file writes an
// unkeyed note; it is spelled here as the empty string because that is what
// [ParseCard] produces for one.
var commentaryKeywords = map[string]bool{
	"COMMENT": true,
	"HISTORY": true,
	"":        true,
}

// formatCard renders one card as exactly [CardSize] bytes.
//
// It returns an error rather than truncating. A truncated card is not a
// slightly-wrong file: the 80-byte grid is the only thing separating one
// record from the next, so a card that overflows shifts every card after it
// and the header stops parsing where the overflow happened. A caller who
// wrote a comment two characters too long deserves to hear about it at the
// call site rather than from someone else's reader a week later.
func formatCard(c Card) ([]byte, error) {
	kw := strings.ToUpper(strings.TrimSpace(c.Keyword))

	if len(kw) > keywordWidth {
		return nil, fmt.Errorf("%w: keyword %q is %d characters, the limit is %d",
			ErrCardTooLong, kw, len(kw), keywordWidth)
	}

	if err := checkPrintable(kw, c); err != nil {
		return nil, err
	}

	var b strings.Builder

	b.Grow(CardSize)
	b.WriteString(kw)
	b.WriteString(strings.Repeat(" ", keywordWidth-len(kw)))

	switch {
	case commentaryKeywords[kw]:
		b.WriteString(c.Comment)

	// END closes a header and carries nothing at all. Writing "END = " would
	// be a value card with no value, which is not what any reader expects to
	// find where the header stops. The same goes for any card a caller left
	// entirely empty: a bare keyword is legal, "KEYWORD = " is not.
	case kw == "END", c.Value == "" && c.Comment == "":

	default:
		b.WriteString(valueIndicator)
		b.WriteString(formatValue(c.Value))

		if c.Comment != "" {
			b.WriteString(" / ")
			b.WriteString(c.Comment)
		}
	}

	out := b.String()
	if len(out) > CardSize {
		return nil, fmt.Errorf("%w: card %q renders to %d characters, the limit is %d",
			ErrCardTooLong, kw, len(out), CardSize)
	}

	return []byte(out + strings.Repeat(" ", CardSize-len(out))), nil
}

// formatValue lays a card's value out in the fixed format.
//
// The value arrives as the text a reader would have parsed — quotes included
// for a string — because that is how [Card] stores it and how [ParseCard]
// produces it. Keeping that representation is what lets a file read by this
// package be written back out unchanged.
func formatValue(value string) string {
	v := strings.TrimSpace(value)

	if v == "" {
		return ""
	}

	// A string value starts at column 11 and is padded to at least eight
	// characters inside its quotes. Left-justified, unlike everything else.
	if v[0] == '\'' {
		return padQuoted(v)
	}

	// Numbers and logicals are right-justified to end at fixedValueEnd. The
	// value begins at column 11, so the padding is what is left of the field.
	const valueStart = keywordWidth + len(valueIndicator) // column 11, 0-based 10

	if width := fixedValueEnd - valueStart; len(v) < width {
		return strings.Repeat(" ", width-len(v)) + v
	}

	return v
}

// padQuoted blank-pads a quoted string's contents to [minStringValue].
//
// Trailing blanks inside a FITS string are not significant, so this changes
// nothing a reader will see; it exists because the standard asks for it and
// because a one-character value written as 'a' has been observed to confuse
// readers that assume the minimum.
func padQuoted(v string) string {
	if len(v) < 2 || v[len(v)-1] != '\'' {
		return v // not a well-formed quoted string; write it as given
	}

	inner := v[1 : len(v)-1]
	if len(inner) >= minStringValue {
		return v
	}

	return "'" + inner + strings.Repeat(" ", minStringValue-len(inner)) + "'"
}

// checkPrintable rejects bytes a FITS header may not contain.
//
// The standard restricts header records to ASCII 32-126. A newline or a tab in
// a comment does not corrupt one card, it corrupts the grid — every reader
// counts 80 bytes, so a stray byte silently shifts the rest of the header into
// nonsense. This is the one place that can still be caught cheaply.
func checkPrintable(kw string, c Card) error {
	for _, part := range []struct {
		what string
		text string
	}{
		{"keyword", kw},
		{"value", c.Value},
		{"comment", c.Comment},
	} {
		for i := range len(part.text) {
			if ch := part.text[i]; ch < 32 || ch > 126 {
				return fmt.Errorf("%w: %s of card %q contains byte %#x at offset %d",
					ErrCardNotPrintable, part.what, kw, ch, i)
			}
		}
	}

	return nil
}
