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

// formatCards renders one card as the 80-byte records a header carries it in.
//
// Usually that is one record. It is more when the card needs a registered
// convention to fit: a keyword over eight characters becomes a HIERARCH card,
// and a string value too long for the remaining columns becomes a value card
// followed by CONTINUE cards. Both are read back as one card by [ParseCard]
// and [ReadHeader], which is what makes the split invisible to a caller.
//
// An over-long card is an error rather than a truncation wherever a convention
// does not apply — to a keyword that is already HIERARCH and still will not
// fit, or to a comment on a numeric card. The 80-byte grid is the only thing
// separating one record from the next, so a card that overflows shifts every
// card after it and the header stops parsing where the overflow happened.
func formatCards(c Card) ([][]byte, error) {
	kw := strings.ToUpper(strings.TrimSpace(c.Keyword))

	if err := checkPrintable(kw, c); err != nil {
		return nil, err
	}

	if commentaryKeywords[kw] {
		return one(formatFixed(kw, "", c.Comment, true))
	}

	if kw == "END" || (c.Value == "" && c.Comment == "") {
		return one(formatFixed(kw, "", "", true))
	}

	if isLongKeyword(kw) {
		return one(formatHierarch(kw, c))
	}

	// A string value that does not fit is split across CONTINUE cards. Only a
	// string can be: the convention is defined for character values alone,
	// which is why a numeric card with an over-long comment is an error.
	if isQuoted(c.Value) {
		if cards, ok, err := formatLongString(kw, c); ok || err != nil {
			return cards, err
		}
	}

	return one(formatFixed(kw, formatValue(c.Value), c.Comment, false))
}

// one wraps a single rendered card, or the error that prevented it.
func one(card []byte, err error) ([][]byte, error) {
	if err != nil {
		return nil, err
	}

	return [][]byte{card}, nil
}

// formatFixed lays out a standard card and pads it to [CardSize].
//
// bare distinguishes a card with no value indicator — END, a commentary
// keyword, or a keyword a caller left entirely empty — from a value card,
// because "KEYWORD = " with nothing after it is not legal where a bare
// keyword is.
func formatFixed(kw, value, comment string, bare bool) ([]byte, error) {
	if len(kw) > keywordWidth {
		return nil, fmt.Errorf("%w: keyword %q is %d characters, the limit is %d",
			ErrCardTooLong, kw, len(kw), keywordWidth)
	}

	var b strings.Builder

	b.Grow(CardSize)
	b.WriteString(kw)
	b.WriteString(strings.Repeat(" ", keywordWidth-len(kw)))

	switch {
	case bare && comment != "":
		b.WriteString(comment)
	case bare:
	default:
		b.WriteString(valueIndicator)
		b.WriteString(value)

		if comment != "" {
			b.WriteString(" / ")
			b.WriteString(comment)
		}
	}

	return padCard(b.String(), kw)
}

// formatHierarch renders a long keyword through the ESO HIERARCH convention.
//
// The layout is "HIERARCH <keyword> = <value>", with no column alignment: the
// keyword has already used the columns a fixed-format value would be aligned
// to, and ESO's own files do not align these.
func formatHierarch(kw string, c Card) ([]byte, error) {
	var b strings.Builder

	b.Grow(CardSize)
	b.WriteString(hierarchPrefix)
	b.WriteString(kw)
	b.WriteString(" = ")
	b.WriteString(strings.TrimSpace(c.Value))

	if c.Comment != "" {
		b.WriteString(" / ")
		b.WriteString(c.Comment)
	}

	out := b.String()
	if len(out) > CardSize {
		// Dropping the comment is the one recovery worth trying: a HIERARCH
		// keyword plus its value is the content, and a comment is not.
		out = hierarchPrefix + kw + " = " + strings.TrimSpace(c.Value)
	}

	return padCard(out, kw)
}

// formatLongString splits a string value across CONTINUE cards.
//
// ok is false when the value fits in one card, so the caller writes it the
// ordinary way rather than invoking a convention for nothing.
//
// Every segment but the last ends with "&" inside its quotes, which is what
// tells a reader another card follows. The comment goes on the final card,
// where the convention puts it and where a reader that stops early still sees
// it belongs to the whole value.
func formatLongString(kw string, c Card) ([][]byte, bool, error) {
	text := unquote(c.Value)

	// What fits on the first card: the record, less the keyword and "= ",
	// less the two quotes.
	const quoteOverhead = 2

	first := CardSize - keywordWidth - len(valueIndicator) - quoteOverhead
	cont := CardSize - len(continueKeyword) - 2 - quoteOverhead

	if len(quoteValue(text))+keywordWidth+len(valueIndicator) <= CardSize && c.Comment == "" {
		return nil, false, nil
	}

	if len(text) <= first-len(" / "+c.Comment) || (c.Comment == "" && len(text) <= first) {
		return nil, false, nil
	}

	segments := splitSegments(text, first-1, cont-1)

	cards := make([][]byte, 0, len(segments))

	for i, seg := range segments {
		last := i == len(segments)-1

		value := seg
		if !last {
			value += string(continuation)
		}

		var (
			card []byte
			err  error
		)

		comment := ""
		if last {
			comment = c.Comment
		}

		if i == 0 {
			card, err = formatFixed(kw, quoteValue(value), comment, false)
		} else {
			card, err = formatContinue(quoteValue(value), comment)
		}

		if err != nil {
			return nil, true, err
		}

		cards = append(cards, card)
	}

	return cards, true, nil
}

// formatContinue renders one CONTINUE card.
//
// CONTINUE takes no value indicator: the standard puts the string in the value
// field directly, which is why ParseCard reads it as a special case rather than
// as an assignment.
func formatContinue(value, comment string) ([]byte, error) {
	var b strings.Builder

	b.Grow(CardSize)
	b.WriteString(continueKeyword)
	b.WriteString("  ")
	b.WriteString(value)

	if comment != "" {
		b.WriteString(" / ")
		b.WriteString(comment)
	}

	return padCard(b.String(), continueKeyword)
}

// splitSegments cuts text into pieces that fit their cards.
func splitSegments(text string, first, rest int) []string {
	if first < 1 {
		first = 1
	}

	if rest < 1 {
		rest = 1
	}

	var (
		out   []string
		width = first
	)

	for len(text) > 0 {
		if len(text) <= width {
			out = append(out, text)

			break
		}

		out = append(out, text[:width])
		text = text[width:]
		width = rest
	}

	return out
}

// padCard blank-fills a rendered card to [CardSize], or reports that it will
// not fit.
func padCard(out, kw string) ([]byte, error) {
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
