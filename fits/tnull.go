package fits

import (
	"math"
	"strconv"
)

// TNULLn is how a FITS binary table says a cell holds no value.
//
// There is no null bitmap in the format. A floating-point column expresses
// absence as NaN, which the IEEE representation already has room for; an
// integer column cannot, so the convention (FITS 4.0 §7.3.2) is to declare one
// stored value as meaning "undefined" and write that value in the empty cells.
//
// The declaration is per column and per file, so the sentinel is a property of
// the table rather than of the type — which is what makes it safe: a column
// whose data genuinely spans the whole integer range simply declares no TNULL
// and has no nulls.

// nullSentinel is the value written into an integer cell that holds nothing.
//
// The most negative value of the column's type. It is the conventional choice —
// cfitsio and astropy both use it — for the reason that makes any choice
// defensible: it has to be a value the column does not otherwise contain, and
// the minimum is the one a physical quantity is least likely to take. A count,
// a flux, an identifier and a pixel index are all far from it.
//
// The writer checks anyway. A column that does contain its own minimum gets no
// TNULL and keeps its values, because reserving a sentinel that collides with
// real data would silently turn measurements into absences — the exact failure
// the convention exists to prevent, inverted.
func nullSentinel(code byte) (int64, bool) {
	switch code {
	case 'B':
		// An unsigned byte column has no negative value to spare; 255 is the
		// conventional sentinel and is checked for collision like the rest.
		return math.MaxUint8, true
	case 'I':
		return math.MinInt16, true
	case 'J':
		return math.MinInt32, true
	case 'K':
		return math.MinInt64, true
	default:
		// Logical and character columns carry their own undefined value — a
		// zero byte and a blank string respectively — and floats use NaN.
		return 0, false
	}
}

// tnullCard builds the TNULLn declaration for a column.
func tnullCard(index int, sentinel int64) Card {
	n := strconv.Itoa(index + 1)

	return Card{
		Keyword: "TNULL" + n,
		Value:   strconv.FormatInt(sentinel, 10),
		Comment: "value used to represent undefined for field " + n,
	}
}

// tnullFor reads a column's declared undefined value.
//
// ok is false when the column declares none, which is the common case and
// means every stored value is real.
func tnullFor(h *Header, index int) (int64, bool) {
	v, err := h.GetInt("TNULL" + strconv.Itoa(index+1))
	if err != nil {
		return 0, false
	}

	return int64(v), true
}
