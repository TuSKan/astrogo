package vizier

import (
	"errors"
	"strings"
	"testing"
)

// A NaN, infinite or impossible position in a VizieR answer is refused as an
// unparseable one has always been (#1080).
//
// strconv.ParseFloat accepts NaN and the infinities, so before #1080 they
// passed this parser's only check and became a target with a NaN coordinate.
// An epoch is read the same way.
func TestParseCSVRefusesANonFinitePosition(t *testing.T) {
	t.Parallel()

	hip := tableSchemas["I/239/hip_main"]
	twoMass := tableSchemas[defaultTable]

	for _, c := range []struct {
		name   string
		schema tableSchema
		body   string
	}{
		{"NaN ra", hip, "designation,ra,dec\n32349,NaN,-16.71\n"},
		{"infinite dec", hip, "designation,ra,dec\n32349,101.28,-Infinity\n"},
		{"dec past the pole", hip, "designation,ra,dec\n32349,101.28,95\n"},
		{"ra in some other unit", hip, "designation,ra,dec\n32349,4000,-16.71\n"},
		{"NaN epoch", twoMass, "designation,ra,dec,jd\n06451281-1642580,101.28,-16.71,NaN\n"},
	} {
		if _, err := parseCSV(strings.NewReader(c.body), c.schema); !errors.Is(err, ErrUnexpectedSchema) {
			t.Errorf("%s: got %v, want ErrUnexpectedSchema", c.name, err)
		}
	}

	// The control: the same row, real.
	got, err := parseCSV(strings.NewReader("designation,ra,dec\n32349,101.28,-16.71\n"), hip)
	if err != nil || len(got) != 1 || !got[0].HasCoord {
		t.Fatalf("a real row: %v, %v", got, err)
	}
}
