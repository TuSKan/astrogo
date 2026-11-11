package iers

import (
	"errors"
	"strings"
	"testing"
)

// A bulletin with no rows is refused, whatever it is instead. Each of these
// parsed into an empty table with no error, and as remote/eop's validator that
// let an error page served with a 200 replace a good bulletin in the cache
// (#1042).
func TestParseRefusesABulletinWithNoRows(t *testing.T) {
	t.Parallel()

	// The bulletin's own last rows: an MJD, and blank polar motion and
	// UT1-UTC. Real bulletins end with these, and on their own they hold no
	// orientation at all.
	blankRow := realRecord[:16] + strings.Repeat(" ", len(realRecord)-16)

	for _, c := range []struct{ name, body string }{
		{"an error page", "<html><body><h1>503 Service Unavailable</h1></body></html>\n"},
		{"an empty file", ""},
		{"blank lines", "\n\n\n"},
		{"only rows with blank fields", blankRow + "\n" + blankRow + "\n"},
	} {
		table, err := ParseFinals2000A(strings.NewReader(c.body))
		if !errors.Is(err, ErrNoRecords) || table != nil {
			t.Errorf("%s: table %v, err %v, want ErrNoRecords", c.name, table, err)
		}
	}

	if table, err := ParseFinals2000A(strings.NewReader(realRecord)); err != nil || len(table.records) != 1 {
		t.Errorf("one real row: err %v, want a table of one record", err)
	}
}
