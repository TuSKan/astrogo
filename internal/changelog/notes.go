package changelog

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Sentinel errors for extracting a release's notes.
var (
	ErrNoSuchRelease = errors.New("changelog: CHANGELOG.md has no section for that version")
	ErrEmptyRelease  = errors.New("changelog: the release's section is empty")
	ErrNotesTooLong  = errors.New("changelog: the release's notes do not fit a GitHub Release")
)

// githubReleaseBodyLimit is the longest body GitHub accepts for a release.
// Its API answers a longer one with a 422, "body is too long (maximum is
// 125000 characters)". v0.20.0's section, 390 entries from 36 days without a
// release, came to 165,662 characters, and its release failed at creation.
// Counted here in bytes, which are never fewer than characters.
const githubReleaseBodyLimit = 125000

// notesPriority is the order sections are kept in when a release's notes do
// not fit: what an upgrader must read first, then what changed, then what was
// added, then what was fixed. A section that does not fit stays as its
// heading and a pointer to CHANGELOG.md. Nothing is cut mid-section, because
// entries wrap across lines and some nest bullets of their own, so there is
// no reliable place to cut one.
var notesPriority = []string{
	"Security",
	"Changed — BREAKING",
	"Removed",
	"Deprecated",
	"Changed",
	"Added",
	"Fixed",
}

// linkReference matches the first line of the link-reference block that
// closes the changelog, e.g. "[0.19.0]: https://...".
var linkReference = regexp.MustCompile(`(?m)^\[[^\]]+\]: \S`)

// ReleaseNotes returns the body of version's section in src: everything
// between its heading, such as "## [0.16.0] — 2026-08-29", and the next
// section heading, or the link references for the oldest section, trimmed.
//
// It is what the release workflow publishes as a GitHub Release's notes, so
// the notes are the changelog the release PR assembled and nothing else. A
// version with no section is ErrNoSuchRelease, and one whose section is
// empty is ErrEmptyRelease: publishing a release with no notes would hide a
// tag pushed without its release PR.
//
// Notes longer than a GitHub Release can hold keep whole sections in
// [notesPriority] order while they fit, and point to CHANGELOG.md at the tag
// for the rest; see [fitNotes].
func ReleaseNotes(src, version string) (string, error) {
	headings := versionHeading.FindAllStringSubmatchIndex(src, -1)

	for i, h := range headings {
		if src[h[2]:h[3]] != version {
			continue
		}

		start := h[1]
		if nl := strings.IndexByte(src[start:], '\n'); nl >= 0 {
			start += nl + 1
		} else {
			start = len(src)
		}

		end := len(src)
		if i+1 < len(headings) {
			end = headings[i+1][0]
		}

		if ref := linkReference.FindStringIndex(src[start:end]); ref != nil {
			end = start + ref[0]
		}

		notes := strings.TrimSpace(src[start:end])
		if notes == "" {
			return "", fmt.Errorf("%w: %s", ErrEmptyRelease, version)
		}

		return fitNotes(notes, version, repositoryURL(src, version))
	}

	return "", fmt.Errorf("%w: %s", ErrNoSuchRelease, version)
}

// notesSection is one "### " section of a release's notes, or the text
// before the first one, which has no name.
type notesSection struct {
	name, body string
}

// fitNotes returns notes unchanged when they fit a GitHub Release. Otherwise
// it keeps whole sections in [notesPriority] order while the result still
// fits, then any section the list does not name, and replaces the body of
// each section left out with a pointer to CHANGELOG.md at version's tag on
// repo. The sections stay in their own order. ErrNotesTooLong is left only
// for notes that do not fit with every section left out.
func fitNotes(notes, version, repo string) (string, error) {
	if len(notes) <= githubReleaseBodyLimit {
		return notes, nil
	}

	where := "CHANGELOG.md at the v" + version + " tag"
	if repo != "" {
		where = fmt.Sprintf("[CHANGELOG.md](%s/blob/v%s/CHANGELOG.md)", repo, version)
	}

	intro := fmt.Sprintf("These notes are longer than a GitHub Release can hold, "+
		"so the sections that say so are in full in %s instead.", where)
	pointer := "Left out here for length: this section is in full in " + where + "."

	sections := splitNotesSections(notes)
	kept := make([]bool, len(sections))

	render := func() string {
		parts := []string{intro}

		for i, s := range sections {
			switch {
			case s.name == "":
				parts = append(parts, s.body)
			case kept[i]:
				parts = append(parts, "### "+s.name+"\n\n"+s.body)
			default:
				parts = append(parts, "### "+s.name+"\n\n"+pointer)
			}
		}

		return strings.Join(parts, "\n\n")
	}

	keep := func(i int) {
		kept[i] = true
		if len(render()) > githubReleaseBodyLimit {
			kept[i] = false
		}
	}

	for _, name := range notesPriority {
		for i, s := range sections {
			if s.name == name {
				keep(i)
			}
		}
	}

	for i, s := range sections {
		if s.name != "" && !slices.Contains(notesPriority, s.name) {
			keep(i)
		}
	}

	out := render()
	if len(out) > githubReleaseBodyLimit {
		return "", fmt.Errorf("%w: %d bytes with every section left out, limit %d",
			ErrNotesTooLong, len(out), githubReleaseBodyLimit)
	}

	return out, nil
}

// splitNotesSections splits notes at their "### " headings.
func splitNotesSections(notes string) []notesSection {
	var (
		sections []notesSection
		cur      notesSection
		body     []string
	)

	flush := func() {
		cur.body = strings.TrimSpace(strings.Join(body, "\n"))
		if cur.name != "" || cur.body != "" {
			sections = append(sections, cur)
		}
	}

	for line := range strings.SplitSeq(notes, "\n") {
		if name, ok := strings.CutPrefix(line, "### "); ok {
			flush()

			cur, body = notesSection{name: strings.TrimSpace(name)}, nil

			continue
		}

		body = append(body, line)
	}

	flush()

	return sections
}

// repositoryURL is the repository the changelog's link for version compares
// on, such as https://github.com/TuSKan/astrogo from
// "[0.20.0]: https://github.com/TuSKan/astrogo/compare/v0.19.0...v0.20.0",
// or "" when there is no such link.
func repositoryURL(src, version string) string {
	m := regexp.MustCompile(`(?m)^\[` + regexp.QuoteMeta(version) + `\]: (\S+)/compare/`).FindStringSubmatch(src)
	if m == nil {
		return ""
	}

	return m[1]
}
