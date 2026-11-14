// Package show prints the examples' output in one style: a title block,
// section headings, aligned label-value lines, and tables.
//
// It is presentation only, not astrogo API. A program copied out of
// examples/ can replace each call with fmt.Printf and lose nothing but the
// alignment.
package show

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Width is the width of every rule the examples draw, in characters.
const Width = 66

// Title prints a program's title block: the title and each subtitle line,
// between two double rules.
func Title(title string, subtitles ...string) { fmt.Print(titleBlock(title, subtitles...)) }

// Section prints a section heading, the name set in a single rule, with a
// blank line before and after.
func Section(name string) { fmt.Print(section(name)) }

// Note prints lines of explanatory text, indented to match the rest.
func Note(lines ...string) { fmt.Print(note(lines...)) }

// Pair is one label and its value, for [Fields].
type Pair struct {
	label, value string
}

// KV is a label and its value, the value formatted as fmt.Sprintf does.
func KV(label, format string, args ...any) Pair {
	return Pair{label: label, value: fmt.Sprintf(format, args...)}
}

// Fields prints pairs as aligned lines: the labels in one column, the
// values starting together after them.
func Fields(pairs ...Pair) { fmt.Print(fields(pairs...)) }

// Column is a table column's title and alignment.
type Column struct {
	title string
	right bool
}

// Left is a column whose cells start together, for names and text.
func Left(title string) Column { return Column{title: title} }

// Right is a column whose cells end together, for numbers.
func Right(title string) Column { return Column{title: title, right: true} }

// Table collects rows and prints them with every column aligned.
type Table struct {
	cols []Column
	rows [][]string
}

// NewTable starts a table with the given columns.
func NewTable(cols ...Column) *Table {
	return &Table{cols: cols}
}

// Row adds a row, one cell per column, each already formatted. A row with
// fewer cells than columns leaves the rest empty; extra cells are dropped.
func (t *Table) Row(cells ...string) {
	t.rows = append(t.rows, cells)
}

// Print prints the table: [Table.String].
func (t *Table) Print() { fmt.Print(t.String()) }

// String is the table as Print prints it: the header, a rule under each
// column, and the rows, indented two spaces with three between columns.
func (t *Table) String() string {
	widths := make([]int, len(t.cols))
	for i, c := range t.cols {
		widths[i] = utf8.RuneCountInString(c.title)
	}

	for _, r := range t.rows {
		for i := range min(len(r), len(widths)) {
			widths[i] = max(widths[i], utf8.RuneCountInString(r[i]))
		}
	}

	header := make([]string, len(t.cols))
	rules := make([]string, len(t.cols))

	for i, c := range t.cols {
		header[i] = c.title
		rules[i] = strings.Repeat("─", widths[i])
	}

	var b strings.Builder

	t.writeRow(&b, header, widths)
	t.writeRow(&b, rules, widths)

	for _, r := range t.rows {
		t.writeRow(&b, r, widths)
	}

	return b.String()
}

func (t *Table) writeRow(b *strings.Builder, cells []string, widths []int) {
	var line strings.Builder

	line.WriteString("  ")

	for i, c := range t.cols {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}

		if i > 0 {
			line.WriteString("   ")
		}

		if c.right {
			line.WriteString(pad(cell, widths[i]) + cell)
		} else {
			line.WriteString(cell + pad(cell, widths[i]))
		}
	}

	// No trailing spaces, whether from a left-aligned last column or a row
	// that stops short.
	b.WriteString(strings.TrimRight(line.String(), " ") + "\n")
}

// titleBlock is the block [Title] prints.
func titleBlock(title string, subtitles ...string) string {
	rule := strings.Repeat("═", Width)

	var b strings.Builder

	b.WriteString(rule + "\n")
	b.WriteString("  " + title + "\n")

	for _, s := range subtitles {
		b.WriteString("  " + s + "\n")
	}

	b.WriteString(rule + "\n")

	return b.String()
}

// section is the heading [Section] prints.
func section(name string) string {
	fill := max(Width-utf8.RuneCountInString(name)-4, 3)
	return fmt.Sprintf("\n── %s %s\n\n", name, strings.Repeat("─", fill))
}

// note is the text [Note] prints.
func note(lines ...string) string {
	var b strings.Builder
	for _, l := range lines {
		b.WriteString("  " + l + "\n")
	}

	return b.String()
}

// fields is the lines [Fields] prints.
func fields(pairs ...Pair) string {
	width := 0
	for _, p := range pairs {
		width = max(width, utf8.RuneCountInString(p.label))
	}

	var b strings.Builder
	for _, p := range pairs {
		b.WriteString("  " + p.label + pad(p.label, width) + "   " + p.value + "\n")
	}

	return b.String()
}

// pad is the spaces that bring s up to width characters.
func pad(s string, width int) string {
	return strings.Repeat(" ", max(width-utf8.RuneCountInString(s), 0))
}
