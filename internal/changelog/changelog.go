// Package changelog assembles CHANGELOG.md from the one-file-per-entry
// fragments in docs/changelog.d.
//
// CHANGELOG.md was the only file five of eight pull requests conflicted on
// in a single batch of parallel work — never the code, always the changelog,
// because every branch appends to the same [Unreleased] section.
//
// Resolving such a conflict textually is not harmless. Merge markers carry
// no information about which heading a bullet belongs under, so an entry
// inherits whichever heading happens to precede it in the merged text: in
// the v0.16.0 batch an "Added" entry silently became a "Fixed" one, in a
// diff that looked like reordering. A file per entry cannot collide, and
// carries its own section.
package changelog

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Sentinel errors for fragment parsing.
var (
	ErrNoFrontMatter = errors.New("changelog: fragment has no --- front matter")
	ErrNoType        = errors.New("changelog: fragment declares no type")
	ErrUnknownType   = errors.New("changelog: unknown type")
	ErrEmptyBody     = errors.New("changelog: fragment has no body")
)

// SectionOrder is the order sections appear in a release, following Keep a
// Changelog with this project's "Changed — BREAKING" variant kept alongside
// plain Changed.
//
//nolint:gochecknoglobals // the canonical section order, read-only
var SectionOrder = []string{
	"Added",
	"Changed — BREAKING",
	"Changed",
	"Deprecated",
	"Removed",
	"Fixed",
	"Security",
}

// Entry is one changelog fragment.
type Entry struct {
	// Type is the section it belongs in, one of [SectionOrder].
	Type string

	// PR is the pull request number, used to order entries within a
	// section so a release reads in the order things landed.
	PR int

	// Body is the entry text, already in the "- **Thing.** Why." shape
	// CHANGELOG.md uses, without the leading bullet.
	Body string

	// Name is the fragment's filename, for error messages.
	Name string
}

// ParseEntry reads one fragment.
//
// The format is deliberately the smallest thing that works: a --- delimited
// header of key: value lines, then the body. Not YAML — the header has two
// keys and pulling in a parser for that would be the tail wagging the dog.
func ParseEntry(name, src string) (Entry, error) {
	e := Entry{Name: name}

	sc := bufio.NewScanner(strings.NewReader(src))
	if !sc.Scan() || strings.TrimSpace(sc.Text()) != "---" {
		return e, fmt.Errorf("%w: %s", ErrNoFrontMatter, name)
	}

	var body strings.Builder

	inHeader := true

	for sc.Scan() {
		line := sc.Text()

		if inHeader {
			if strings.TrimSpace(line) == "---" {
				inHeader = false

				continue
			}

			key, value, found := strings.Cut(line, ":")
			if !found {
				continue
			}

			switch strings.TrimSpace(key) {
			case "type":
				e.Type = strings.TrimSpace(value)
			case "pr":
				e.PR, _ = strconv.Atoi(strings.TrimSpace(value))
			}

			continue
		}

		body.WriteString(line)
		body.WriteString("\n")
	}

	if err := sc.Err(); err != nil {
		return e, fmt.Errorf("changelog: read %s: %w", name, err)
	}

	if inHeader {
		return e, fmt.Errorf("%w: %s", ErrNoFrontMatter, name)
	}

	if e.Type == "" {
		return e, fmt.Errorf("%w: %s", ErrNoType, name)
	}

	if !validType(e.Type) {
		return e, fmt.Errorf("%w %q in %s (want one of %s)", ErrUnknownType, e.Type, name, strings.Join(SectionOrder, ", "))
	}

	e.Body = strings.TrimSpace(body.String())
	if e.Body == "" {
		return e, fmt.Errorf("%w: %s", ErrEmptyBody, name)
	}

	return e, nil
}

func validType(t string) bool { return slices.Contains(SectionOrder, t) }

// LoadEntries parses every .md fragment in dir, skipping README.md.
//
// Every fragment is parsed even after one fails, so a run reports all the
// malformed files rather than only the first — the difference between one
// fix and one fix per iteration.
func LoadEntries(dir fs.FS) ([]Entry, error) {
	names, err := fs.Glob(dir, "*.md")
	if err != nil {
		return nil, fmt.Errorf("changelog: list fragments: %w", err)
	}

	var (
		entries []Entry
		errs    []error
	)

	for _, name := range names {
		if strings.EqualFold(path.Base(name), "README.md") {
			continue
		}

		raw, rerr := fs.ReadFile(dir, name)
		if rerr != nil {
			errs = append(errs, fmt.Errorf("changelog: read %s: %w", name, rerr))

			continue
		}

		e, perr := ParseEntry(name, string(raw))
		if perr != nil {
			errs = append(errs, perr)

			continue
		}

		entries = append(entries, e)
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return entries, nil
}

// citation matches a trailing pull request reference such as "(#61)" or
// "(#56, #57)".
var citation = regexp.MustCompile(`\(#\d+(, ?#\d+)*\)\.?$`)

// cite returns the entry body with its pull request appended, in the citation
// style CHANGELOG.md uses throughout.
//
// The pr field used to be read for ordering and then dropped, so the first
// assembled release carried eleven entries with no way to reach the change
// behind them, beside two hand-written ones that cited theirs. A changelog is
// an index, and an index entry with no pointer is decoration.
//
// A body that already ends in a citation keeps it, so an entry naming two pull
// requests — a fix and the follow-up that completed it — is not rewritten to
// name only one.
func (e Entry) cite() string {
	if e.PR == 0 || citation.MatchString(e.Body) {
		return e.Body
	}

	return fmt.Sprintf("%s (#%d)", strings.TrimRight(e.Body, " "), e.PR)
}

// Render turns entries into a CHANGELOG section body, headings in
// [SectionOrder] and entries within a heading ordered by PR number.
//
// Returns the empty string for no entries, so a caller can tell "nothing to
// release" from "a release with no notes".
func Render(entries []Entry) string {
	if len(entries) == 0 {
		return ""
	}

	bySection := make(map[string][]Entry, len(SectionOrder))
	for _, e := range entries {
		bySection[e.Type] = append(bySection[e.Type], e)
	}

	var b strings.Builder

	for _, section := range SectionOrder {
		in := bySection[section]
		if len(in) == 0 {
			continue
		}

		sort.SliceStable(in, func(i, j int) bool { return in[i].PR < in[j].PR })

		b.WriteString("\n### ")
		b.WriteString(section)
		b.WriteString("\n")

		for _, e := range in {
			b.WriteString("- ")
			b.WriteString(e.cite())
			b.WriteString("\n")
		}
	}

	return b.String()
}
