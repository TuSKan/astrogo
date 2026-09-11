package fits

import (
	"strings"
)

// The two registered conventions that let a FITS header carry what the fixed
// 80-byte record cannot: a keyword longer than eight characters, and a string
// value longer than sixty-eight.
//
// Both are old, both are everywhere, and neither is optional in practice. ESO
// instruments emit HIERARCH by the dozen per header; any pipeline that records
// a file path or a long provenance string emits CONTINUE. A reader without
// them does not fail loudly — it silently returns a mangled keyword or a
// truncated value, which is the worse outcome.
//
// References:
//   - HIERARCH: ESO, "The ESO HIERARCH Keyword Convention" (ESO-DICB), and
//     FITS 4.0 §4.1.2.1, which registers it.
//   - CONTINUE: FITS 4.0 §4.2.1.2, the long-string convention originally
//     published as OGIP 100 / "OGIP 1.0", announced by LONGSTRN.

const (
	// hierarchPrefix introduces a long keyword. The trailing space is part of
	// it: "HIERARCHY" is an ordinary eight-character keyword, not a HIERARCH
	// card.
	hierarchPrefix = "HIERARCH "

	// continueKeyword carries the next piece of a long string value.
	continueKeyword = "CONTINUE"

	// longStringMarker is the value LONGSTRN takes to announce that CONTINUE
	// cards are in use, so a reader that does not implement the convention has
	// something to detect rather than a value that silently ends early.
	longStringMarker = "'OGIP 1.0'"

	// continuation is the character a long string's non-final segment ends
	// with, inside the quotes.
	continuation = '&'
)

// isLongKeyword reports whether kw needs the HIERARCH convention.
//
// Over eight characters is the usual reason. A keyword carrying a character
// the standard does not allow in columns 1-8 — anything but A-Z, 0-9, hyphen
// and underscore — is the other, and HIERARCH is where those go too.
func isLongKeyword(kw string) bool {
	if len(kw) > keywordWidth {
		return true
	}

	for i := range len(kw) {
		c := kw[i]

		switch {
		case c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return true
		}
	}

	return false
}

// parseHierarch splits a HIERARCH card into its keyword and the rest.
//
// The keyword is the tokens between "HIERARCH " and the first "=" outside a
// quoted string, with runs of whitespace collapsed to one space — which is how
// ESO writes them and how astropy normalises them, so that "ESO  DET" and
// "ESO DET" are the same keyword rather than two.
//
// ok is false for a card that is not HIERARCH, or is HIERARCH with no "=",
// which the ESO convention does not define and which is left to be read as an
// ordinary card rather than guessed at.
func parseHierarch(s string) (keyword, rest string, ok bool) {
	if !strings.HasPrefix(s, hierarchPrefix) {
		return "", "", false
	}

	body := s[len(hierarchPrefix):]

	eq := indexOutsideQuotes(body, '=')
	if eq < 0 {
		return "", "", false
	}

	keyword = strings.Join(strings.Fields(body[:eq]), " ")
	if keyword == "" {
		return "", "", false
	}

	return keyword, body[eq+1:], true
}

// indexOutsideQuotes returns the first index of c that is not inside a FITS
// quoted string, or -1.
//
// A doubled quote is FITS's escape for a literal one, so it must not flip the
// state — without that, a value like 'O”Brien' leaves this function thinking
// it is outside a string when it is inside, and the comment separator is then
// found in the middle of the text.
func indexOutsideQuotes(s string, c byte) int {
	inQuote := false

	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '\'':
			if inQuote && i+1 < len(s) && s[i+1] == '\'' {
				i++ // an escaped quote, not the end of the string

				continue
			}

			inQuote = !inQuote
		case s[i] == c && !inQuote:
			return i
		}
	}

	return -1
}

// splitValueComment separates a card's value from its trailing comment.
//
// The separator is the first "/" outside a quoted string. Everything before it
// is the value as written — quotes included, because that is how [Card] stores
// a string and what lets a card read from a file be written back unchanged.
func splitValueComment(rest string) (value, comment string) {
	slash := indexOutsideQuotes(rest, '/')
	if slash < 0 {
		return strings.TrimSpace(rest), ""
	}

	return strings.TrimSpace(rest[:slash]), strings.TrimSpace(rest[slash+1:])
}

// isQuoted reports whether v is a FITS string value.
func isQuoted(v string) bool {
	return len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\''
}

// unquote returns the text inside a FITS string value, with doubled quotes
// collapsed to one and trailing blanks removed.
//
// Trailing blanks are not significant in a FITS string — the standard says so
// explicitly, and every writer pads short values out to eight characters — so
// keeping them would make 'M31' and 'M31     ' different strings.
func unquote(v string) string {
	if !isQuoted(v) {
		return strings.TrimSpace(v)
	}

	return strings.TrimRight(strings.ReplaceAll(v[1:len(v)-1], "''", "'"), " ")
}

// QuoteValue renders s as a FITS string card value, doubling any quote inside
// it as the standard requires.
//
// [Card.Value] holds a value as it appears in the file, so a string one carries
// its own quotes. This is how to build one without knowing that:
//
//	h.Append(fits.Card{Keyword: "OBJECT", Value: fits.QuoteValue("M31")})
//
// A caller who writes the quotes by hand and forgets to double an apostrophe
// produces a card that ends early, with the rest read as a comment.
func QuoteValue(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// quoteValue is the internal spelling, kept so the rest of this package reads
// the same as it did before the function was exported.
func quoteValue(s string) string { return QuoteValue(s) }

// joinContinuation appends a CONTINUE card's contribution to a card already
// holding a long string.
//
// The convention is that every segment but the last ends with "&" inside its
// quotes; this drops that marker and concatenates. A comment on any
// continuation card belongs to the whole value, so the last one seen wins —
// which is what writers produce, since the comment goes on the final card.
//
// ok is false when prev is not waiting for a continuation, so a stray CONTINUE
// card is kept as itself rather than silently glued onto whatever preceded it.
func joinContinuation(prev *Card, cont Card) bool {
	if prev == nil || !isQuoted(prev.Value) {
		return false
	}

	inner := prev.Value[1 : len(prev.Value)-1]
	if !strings.HasSuffix(inner, string(continuation)) {
		return false
	}

	next := cont.Value
	if !isQuoted(next) {
		return false
	}

	prev.Value = "'" + inner[:len(inner)-1] + next[1:len(next)-1] + "'"

	if cont.Comment != "" {
		prev.Comment = cont.Comment
	}

	return true
}
