package changelog

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Sentinel errors for extracting a release's notes.
var (
	ErrNoSuchRelease = errors.New("changelog: CHANGELOG.md has no section for that version")
	ErrEmptyRelease  = errors.New("changelog: the release's section is empty")
)

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

		return notes, nil
	}

	return "", fmt.Errorf("%w: %s", ErrNoSuchRelease, version)
}
