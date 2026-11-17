package show

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTitleBlock(t *testing.T) {
	got := titleBlock("Mars", "from Quinta Calixto")
	rule := strings.Repeat("═", Width)

	want := rule + "\n  Mars\n  from Quinta Calixto\n" + rule + "\n"
	if got != want {
		t.Errorf("titleBlock =\n%q\nwant\n%q", got, want)
	}
}

func TestSectionFillsTheRule(t *testing.T) {
	got := section("Rise and set")

	if !strings.HasPrefix(got, "\n── Rise and set ─") || !strings.HasSuffix(got, "\n\n") {
		t.Errorf("section = %q", got)
	}

	if n := utf8.RuneCountInString(strings.Trim(got, "\n")); n != Width {
		t.Errorf("section line is %d characters, want %d: %q", n, Width, got)
	}

	// A name longer than the rule still gets a short rule, not a panic.
	long := strings.Repeat("x", Width)
	if got := section(long); !strings.Contains(got, long+" ───") {
		t.Errorf("section(long) = %q", got)
	}
}

func TestNoteIndents(t *testing.T) {
	if got, want := note("one", "two"), "  one\n  two\n"; got != want {
		t.Errorf("note = %q, want %q", got, want)
	}
}

func TestFieldsAlignValues(t *testing.T) {
	got := fields(KV("Target", "%s", "Sirius"), KV("Altitude", "%.1f°", 45.25))

	want := "  Target     Sirius\n  Altitude   45.2°\n"
	if got != want {
		t.Errorf("fields =\n%q\nwant\n%q", got, want)
	}
}

func TestTableAlignsByColumn(t *testing.T) {
	tab := NewTable(Left("Target"), Right("Score"), Left("Note"))
	tab.Row("Omega Centauri", "53.9", "")
	tab.Row("Canopus", "0.0", "below 20°")
	tab.Row("Sgr A*") // short row: the rest stay empty

	want := strings.Join([]string{
		"  Target           Score   Note",
		"  ──────────────   ─────   ─────────",
		"  Omega Centauri    53.9",
		"  Canopus            0.0   below 20°",
		"  Sgr A*",
		"",
	}, "\n")

	if got := tab.String(); got != want {
		t.Errorf("table =\n%s\nwant\n%s", got, want)
	}
}

func TestTableCountsCharactersNotBytes(t *testing.T) {
	tab := NewTable(Left("Body"), Right("Alt"))
	tab.Row("α Cen", "41.9°")
	tab.Row("Vega", "26.6°")

	// The right-aligned last column ends every line at the same character.
	lines := strings.Split(strings.TrimSuffix(tab.String(), "\n"), "\n")
	for _, l := range lines {
		if n, want := utf8.RuneCountInString(l), utf8.RuneCountInString(lines[1]); n != want {
			t.Errorf("line %q is %d characters, want %d like the rule", l, n, want)
		}
	}
}
