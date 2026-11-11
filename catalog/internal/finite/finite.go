// Package finite reads the numbers in a catalog answer under one rule: a
// value that is not finite is not a value (#1080).
//
// # Why one package for it
//
// strconv.ParseFloat accepts "NaN", "Inf" and "Infinity", and VOTable allows
// NaN for a null float, so every catalog parser that read numbers with it
// kept them. Downstream nothing fails: a target whose right ascension is NaN
// is "not observable" to every constraint but one, has no rise or set, and
// drops out of a ranking and a schedule without a word, so a broken catalog
// row reads exactly like a star below the horizon.
//
// The rule is that a non-finite number is read exactly as an empty or
// unparseable one already is, and each parser then does what it already does
// with an absent value: one drops the row, another keeps the target with no
// position. Eight parsers read numbers; one reader keeps them agreeing.
package finite

import (
	"math"
	"strconv"
)

// Parse reads s as a number, and reports false when it is not one: when it
// does not parse, or when it parses to NaN or an infinity. The text is taken
// as given, without trimming, so a caller that trimmed before still trims.
func Parse(s string) (float64, bool) {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || !Is(v) {
		return 0, false
	}

	return v, true
}

// Is reports whether v is neither NaN nor infinite, for a number that arrives
// already decoded rather than as text.
func Is(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// Position reports whether a right ascension and declination in degrees can
// be a position at all: both finite, the declination within ±90°, and the
// right ascension within ±360°, which takes both the 0–360 and the ±180
// conventions.
//
// A position that fails is no position. It is not repaired: a declination of
// 95° is not a declination, and a right ascension of 4000 is one written in
// some other unit, and guessing which would be fabricating a coordinate.
func Position(raDeg, decDeg float64) bool {
	return Is(raDeg) && Is(decDeg) &&
		decDeg >= -90 && decDeg <= 90 &&
		raDeg >= -360 && raDeg <= 360
}
