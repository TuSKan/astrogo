package lsk_test

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/jpl/lsk"
)

var errShortRead = errors.New("connection reset mid-kernel")

// failingReader yields prefix, then fails.
type failingReader struct {
	prefix io.Reader
	err    error
}

func (f *failingReader) Read(p []byte) (int, error) {
	n, err := f.prefix.Read(p)
	if errors.Is(err, io.EOF) {
		return n, f.err
	}

	if err != nil {
		return n, fmt.Errorf("failingReader: %w", err)
	}

	return n, nil
}

func (f *failingReader) Close() error { return nil }

// TestNewReaderReportsAReadFailure pins that a kernel which stops arriving
// part-way through is an error rather than a short table.
//
// # Why this is the dangerous case
//
// A total failure was already caught: no entries parsed means ErrNoLeapseconds.
// A *partial* read was not. The scanner stopped, the entries seen so far were
// kept, and NewReader returned a Reader holding a table that simply ends early
// — with no indication anything went wrong.
//
// That is the same defect a dropped final entry produced once before in this
// package, where the table ended at 36 leap seconds and every UTC epoch from
// 2017 onward converted one second early, putting the geocentric Sun about
// 30 km from where DE440 has it. A truncated download reaches it by a different
// route, and the emptiness check cannot see it: the prefix below parses
// perfectly well and yields a plausible, wrong table.
func TestNewReaderReportsAReadFailure(t *testing.T) {
	t.Parallel()

	// Valid kernel text, cut off after the first few DELTA_AT entries.
	const truncated = `\begindata

DELTET/DELTA_T_A       =   32.184
DELTET/K               =    1.657D-3
DELTET/EB              =    1.671D-2
DELTET/M               = (  6.239996D0   1.99096871D-7 )

DELTET/DELTA_AT        = ( 10,   @1972-JAN-1
                           11,   @1972-JUL-1
                           12,   @1973-JAN-1
`

	r, err := lsk.NewReader(&failingReader{prefix: strings.NewReader(truncated), err: errShortRead})
	if !errors.Is(err, errShortRead) {
		t.Errorf("err = %v, want the underlying read failure — a truncated kernel "+
			"must not parse into a short table", err)
	}

	if r != nil {
		t.Error("a Reader was returned alongside a read failure")
	}
}

// TestNewReaderAcceptsAWholeKernel is the control: the same text, delivered
// completely, parses. Without it the test above would pass on a reader that
// rejected everything.
func TestNewReaderAcceptsAWholeKernel(t *testing.T) {
	t.Parallel()

	const whole = `\begindata

DELTET/DELTA_T_A       =   32.184
DELTET/K               =    1.657D-3
DELTET/EB              =    1.671D-2
DELTET/M               = (  6.239996D0   1.99096871D-7 )

DELTET/DELTA_AT        = ( 10,   @1972-JAN-1
                           11,   @1972-JUL-1
                           12,   @1973-JAN-1 )

\begintext
`

	r, err := lsk.NewReader(io.NopCloser(strings.NewReader(whole)))
	if err != nil {
		t.Fatalf("a complete kernel failed to parse: %v", err)
	}

	if got := len(r.DeltaAt); got != 3 {
		t.Errorf("parsed %d DELTA_AT entries, want 3", got)
	}

	if r.K != 1.657e-3 {
		t.Errorf("K = %v, want 1.657e-3 — the Fortran exponent form was not handled", r.K)
	}
}
