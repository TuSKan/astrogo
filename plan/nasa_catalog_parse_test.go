//go:build integration

package plan_test

import (
	"errors"
	"testing"
)

// TestNASACatalogParsersKeepEveryTypeTheKeyDefines parses one real row for each
// type code the parsers used to drop (#398), copied byte-exact from the catalog
// pages the eclipse tests read, and checks each is kept, with its type letter.
//
// The rows are the marginal and non-central cases: a total penumbral (Nx), the
// first and last penumbrals of a saros (Nb, Ne), hybrids that begin total and
// that begin annular (H2, H3), non-central annulars and totals (A+, A-, T-),
// central eclipses with a limit off the Earth (An, Tn), the first and last
// partials of a saros (Pb, Pe). T+, T-, Hm and As were kept before and are here
// so that the change is seen not to lose them.
func TestNASACatalogParsersKeepEveryTypeTheKeyDefines(t *testing.T) {
	lunar := []struct {
		row, letter string
	}{
		{"05064  0096 Apr 26  09:26:31   9588 -23546   52   Nx  -t  -1.0177  1.0262 -0.0444  285.9    -      -     14S  103W", "N"},
		{"04897  0030 May 06  06:10:02  10231 -24362   41   Ne  -a   1.5348  0.0122 -0.9295   29.4    -      -     14S   51W", "N"},
		{"04919  0038 Jul 05  07:09:37  10150 -24261   88   Nb  h-   1.5077  0.0952 -0.9125   88.4    -      -     22S   65W", "N"},
		{"04830  0003 Oct 29  01:32:40  10496 -24690   65   T+  pp   0.0645  2.7919  1.6883  378.7  234.8  103.9   13N   17E", "T"},
		{"04829  0003 May 04  22:22:05  10501 -24696   60   T-  -p  -0.1939  2.4752  1.5284  316.7  208.7   91.3   15S   67E", "T"},
	}

	for _, c := range lunar {
		refs := parseNASALunarEclipses(t, c.row)
		if len(refs) != 1 {
			t.Errorf("lunar row %q: parsed %d eclipses, want 1", c.row, len(refs))

			continue
		}

		if refs[0].EclipseType != c.letter {
			t.Errorf("lunar row %q: type %q, want %q", c.row, refs[0].EclipseType, c.letter)
		}
	}

	solar := []struct {
		row, letter string
	}{
		{"04861  0035 Aug 21  21:43:50  10178 -24296   84   H2  p-  -0.6201  1.0130  22S 120W  52   56  01m13s", "H"},
		{"05237  0190 Oct 16  20:54:58   8688 -22377   85   H3  n-   0.4293  1.0152  16N  93W  64   57  01m30s", "H"},
		{"04874  0040 Apr 30  01:03:17  10132 -24238   58   A+  -t   1.0100  0.9522  70N  70E   0             ", "A"},
		{"06181  0596 Jul 01  10:54:33   4749 -17359  104   A-  t-  -0.9988  0.9827  65S  11E   0             ", "A"},
		{"05059  0115 May 11  20:48:54   9405 -23310   88   T-  t-  -1.0051  1.0117  70S  62W   0             ", "T"},
		{"04817  0018 Feb 04  20:20:34  10353 -24513   89   Pb  t-   1.5494  0.0205  62N 133W   0             ", "P"},
		{"04852  0032 Mar 29  23:10:01  10212 -24338   49   Pe  -t  -1.4877  0.1002  61S  39W   0             ", "P"},
		{"05080  0123 Nov 06  03:01:20   9324 -23205   64   An  -p   0.9783  0.9098  54N 148W  11   -   08m20s", "A"},
		{"08366  1523 Aug 11  04:33:16    173  -5892  108   Tn  -t   0.9969  1.0558  63N 136W   2   -   02m44s", "T"},
		{"04830  0023 Apr 09  00:41:09  10301 -24449   68   Hm  nn   0.1404  1.0082  14N 149W  82   29  00m51s", "H"},
		{"04916  0057 Jun 20  20:59:17   9964 -24026   86   As  t-  -0.9809  0.9434  56S 101W  10   -   05m25s", "A"},
	}

	for _, c := range solar {
		refs := parseNASASolarEclipses(t, c.row)
		if len(refs) != 1 {
			t.Errorf("solar row %q: parsed %d eclipses, want 1", c.row, len(refs))

			continue
		}

		if refs[0].EclipseType != c.letter {
			t.Errorf("solar row %q: type %q, want %q", c.row, refs[0].EclipseType, c.letter)
		}
	}
}

// TestNASACatalogTypeOutsideTheKeyIsMalformed: a type field the catalog's key
// does not define is a malformed row, not a row to leave out. Each catalog's
// codes are refused by the other's key where they differ, and a qualifier is
// one character at most.
func TestNASACatalogTypeOutsideTheKeyIsMalformed(t *testing.T) {
	for _, c := range []struct {
		key    string
		fields []string
	}{
		{"lunar", []string{"A", "H", "Pn", "T2", "Nm+", "", "n", "-"}},
		{"solar", []string{"N", "Nx", "T*", "Hx", "Pmm", "", "t", "+"}},
	} {
		key := lunarType
		if c.key == "solar" {
			key = solarType
		}

		for _, f := range c.fields {
			letter, err := eclipseType(key, f)
			if !errors.Is(err, errCatalogField) {
				t.Errorf("%s type %q: letter %q, err %v; want errCatalogField", c.key, f, letter, err)
			}
		}
	}
}
