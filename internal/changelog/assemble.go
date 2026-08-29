package changelog

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Sentinel errors for assembling a release.
var (
	ErrNoUnreleased     = errors.New("changelog: CHANGELOG.md has no [Unreleased] heading")
	ErrVersionExists    = errors.New("changelog: version already released")
	ErrNoUnreleasedLink = errors.New("changelog: CHANGELOG.md has no [Unreleased] link reference")
)

const unreleasedHeading = "## [Unreleased]"

// versionHeading matches a released section heading, e.g.
// "## [0.16.0] — 2026-08-29".
var versionHeading = regexp.MustCompile(`(?m)^## \[([0-9]+\.[0-9]+\.[0-9]+)\]`)

// unreleasedLink matches the link reference [Unreleased] resolves through,
// whose target names the most recent tag.
var unreleasedLink = regexp.MustCompile(`(?m)^\[Unreleased\]: (\S+/compare/v)([0-9]+\.[0-9]+\.[0-9]+)(\.\.\.HEAD)$`)

// Assemble inserts a new release section built from entries, leaving
// [Unreleased] empty and extending the link-reference chain.
//
// Released sections are never rewritten — this changelog is forward-only —
// so everything below the new section is copied verbatim.
//
// Anything already written by hand under [Unreleased] is folded into the
// release alongside the fragments, merged by heading. Appending the two
// bodies instead would produce a second "### Fixed" under one release,
// which is the same silently-wrong output that motivated changelog.d in the
// first place; and dropping them would lose entries written before the
// convention existed.
func Assemble(src, version, date string, entries []Entry) (string, error) {
	if !strings.Contains(src, unreleasedHeading) {
		return "", ErrNoUnreleased
	}

	for _, m := range versionHeading.FindAllStringSubmatch(src, -1) {
		if m[1] == version {
			return "", fmt.Errorf("%w: %s", ErrVersionExists, version)
		}
	}

	link := unreleasedLink.FindStringSubmatch(src)
	if link == nil {
		return "", ErrNoUnreleasedLink
	}

	head, existing, tail := splitUnreleased(src)

	body := mergeSections(existing, Render(entries))

	out := head + unreleasedHeading + "\n\n## [" + version + "] — " + date + "\n" + body + "\n" + tail

	// Point [Unreleased] at the new tag, and add this version's own link
	// directly beneath it to match the existing newest-first chain.
	compareURL, previous := link[1], link[2]
	newLinks := "[Unreleased]: " + compareURL + version + "...HEAD\n" +
		"[" + version + "]: " + compareURL + previous + "...v" + version

	return strings.Replace(out, link[0], newLinks, 1), nil
}

// splitUnreleased returns everything before the [Unreleased] heading, the
// body between it and the next "## " heading, and everything from that
// heading on. With no [Unreleased] heading the whole input is the head,
// a case Assemble refuses before reaching here.
func splitUnreleased(src string) (head, body, tail string) {
	head, rest, found := strings.Cut(src, unreleasedHeading)
	if !found {
		return src, "", ""
	}

	body, tail, found = strings.Cut(rest, "\n## ")
	if !found {
		return head, rest, ""
	}

	return head, body, "## " + tail
}

// mergeSections combines two rendered bodies, keeping one heading each and
// preserving the canonical section order.
func mergeSections(bodies ...string) string {
	buckets := make(map[string][]string, len(SectionOrder))

	var order []string

	for _, body := range bodies {
		var current string

		for line := range strings.SplitSeq(body, "\n") {
			switch {
			case strings.HasPrefix(line, "### "):
				current = strings.TrimSpace(strings.TrimPrefix(line, "### "))
				if _, seen := buckets[current]; !seen {
					buckets[current] = nil

					order = append(order, current)
				}
			case strings.TrimSpace(line) == "" || current == "":
				// Blank lines are re-emitted below; text before the first
				// heading is preamble and is not an entry.
			default:
				buckets[current] = append(buckets[current], line)
			}
		}
	}

	var b strings.Builder

	for _, section := range sortSections(order) {
		if len(buckets[section]) == 0 {
			continue
		}

		b.WriteString("\n### ")
		b.WriteString(section)
		b.WriteString("\n")

		for _, line := range buckets[section] {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	return b.String()
}

// sortSections puts known sections in [SectionOrder], then any unrecognised
// ones in the order they were found — a heading this package does not know
// about is still somebody's entry, and dropping it would be worse than
// filing it last.
func sortSections(found []string) []string {
	out := make([]string, 0, len(found))
	seen := make(map[string]bool, len(found))

	for _, section := range SectionOrder {
		for _, f := range found {
			if f == section && !seen[f] {
				out = append(out, f)
				seen[f] = true
			}
		}
	}

	for _, f := range found {
		if !seen[f] {
			out = append(out, f)
			seen[f] = true
		}
	}

	return out
}
