// Package mpcorb reads the IAU Minor Planet Center's orbital-element files.
//
// MPCORB is the standard offline small-body path: one file, every numbered and
// multi-opposition object in it, full published precision. astrogo could
// already resolve a small body through [github.com/TuSKan/astrogo/catalog/sbdb]
// and propagate it with [github.com/TuSKan/astrogo/ephemeris/kepler], but only
// one object per API call — which is not an answer to "plan a night around
// five hundred asteroids", and SBDB returns its elements rounded to three
// significant figures besides.
//
// # What this package does not do
//
// Comets. The MPC publishes them in CometEls.txt, in a different format built
// around perihelion distance and time rather than semi-major axis and mean
// anomaly, and most of them are near-parabolic: e is at or above 1 for the
// long-period comets that make up the bulk of the file. astrogo's propagator is
// elliptical only ([github.com/TuSKan/astrogo/ephemeris/kepler.ErrUnsupportedOrbit]),
// so a CometEls reader would parse a file it cannot propagate. The propagator
// comes first; see #128.
//
// # Sizes
//
// Measured 2026-09-08. MPCORB.DAT holds everything; the cuts of it are two to
// three orders of magnitude smaller and are usually what a caller actually
// wants:
//
//	MPCORB.DAT     everything                     317 MB
//	MPCORB.DAT.gz  the same, compressed            94 MB
//	NEA.txt        near-Earth asteroids           8.6 MB
//	Unusual.txt    unusual orbits                 8.6 MB
//	Distant.txt    Centaurs and trans-Neptunians  1.7 MB
//	PHA.txt        potentially hazardous          0.5 MB
//
// [Open] streams whichever one is named, so a caller filtering for five hundred
// objects never holds a million.
package mpcorb

import (
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"iter"
	"strconv"
	"strings"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

// Open fetches an MPCORB-format file from [remote.MPCORB] and streams its
// rows.
//
// name is the file under the MPC's MPCORB directory — "MPCORB.DAT",
// "NEA.txt", "Distant.txt", "PHA.txt", "Unusual.txt". It is
// [remote.Downloadable], so the caller must have granted
// [remote.EnableDownloads] for [remote.MPCORB]; because the sizes span two
// orders of magnitude the endpoint declares [remote.SizeVaries], which means
// the grant's own byte budget is what decides.
//
// The fetch happens here, so an unreachable endpoint or a denied download is
// an error from this call rather than a surprise on the first iteration. The
// file is opened when iteration starts and closed when it ends, including on
// an early break, so a caller taking the first fifty rows of the 317 MB file
// pays for fifty rows.
func Open(ctx context.Context, name string, opts ...resolve.Option) (iter.Seq2[resolve.Target, error], error) {
	bucket, key, err := clientOf(opts).GetFile(ctx, remote.MPCORB, name)
	if err != nil {
		return nil, fmt.Errorf("mpcorb: fetch %s: %w", name, err)
	}

	return func(yield func(resolve.Target, error) bool) {
		r, err := bucket.NewReader(ctx, key, nil)
		if err != nil {
			yield(resolve.Target{}, fmt.Errorf("mpcorb: open %s: %w", name, err))

			return
		}

		defer r.Close() //nolint:errcheck // read-only handle, nothing actionable on close failure

		for t, err := range Read(r) {
			if !yield(t, err) {
				return
			}
		}
	}, nil
}

// Read streams the rows of an MPCORB-format file, for a caller holding one
// already — a local copy, an archived snapshot, a file from a source other
// than the MPC's own directory.
//
// gzip is detected from the stream's first two bytes rather than from a name,
// so MPCORB.DAT.gz works without being told and a file misnamed either way
// still reads.
//
// # Errors are yielded, not returned
//
// A malformed row yields a zero Target and an error and iteration continues.
// That is the opposite of what [github.com/TuSKan/astrogo/plan]'s MPC
// observatory reader does, deliberately: that file is ~2,700 rows and is the
// authority on which codes exist, so one bad row means the fetch is wrong.
// This one is over 1.5 million rows maintained by many hands, and refusing
// the whole catalogue over a single object nobody asked for would be the
// wrong trade. A caller that wants the strict reading collects the errors and
// stops; a caller planning a night skips them.
//
// Reading stops at the first *read* error, which is a different thing from a
// bad row and is yielded as such.
func Read(r io.Reader) iter.Seq2[resolve.Target, error] {
	return func(yield func(resolve.Target, error) bool) {
		br := bufio.NewReader(r)

		if magic, err := br.Peek(2); err == nil && magic[0] == 0x1f && magic[1] == 0x8b {
			gz, err := gzip.NewReader(br)
			if err != nil {
				yield(resolve.Target{}, fmt.Errorf("mpcorb: gzip: %w", err))

				return
			}

			defer gz.Close() //nolint:errcheck // read-only handle, nothing actionable on close failure

			br = bufio.NewReader(gz)
		}

		sc := bufio.NewScanner(br)

		// A row is 202 bytes; bufio.Scanner's default 64 KiB ceiling is ample,
		// but the file's header lines are of no fixed length and a corrupted
		// stream can present one very long "line".
		sc.Buffer(make([]byte, 0, 4096), 1<<20)

		line := 0

		for sc.Scan() {
			line++

			row := strings.TrimRight(sc.Text(), "\r\n")

			if !isElementRow(row) {
				continue
			}

			t, err := parseRow(row)
			if err != nil {
				if !yield(resolve.Target{}, fmt.Errorf("line %d: %w", line, err)) {
					return
				}

				continue
			}

			if !yield(t, nil) {
				return
			}
		}

		if err := sc.Err(); err != nil {
			yield(resolve.Target{}, fmt.Errorf("mpcorb: read: %w", err))
		}
	}
}

// MPCORB export-format column bounds, as half-open Go slice indices. The
// published format numbers columns from 1 and inclusively; these are those
// numbers minus one, and the pairing is checked against 50,564 real rows in
// NEA.txt and Distant.txt.
//
// Parsed by column and never by splitting on whitespace: several fields carry
// no separator when a value fills its width, and the readable designation is a
// name with spaces in it.
const (
	colDesigEnd    = 7   //   1-7    packed designation
	colHStart      = 8   //   9-13   absolute magnitude H
	colHEnd        = 13  //
	colGStart      = 14  //  15-19   slope parameter G
	colGEnd        = 19  //
	colEpochStart  = 20  //  21-25   epoch of osculation, packed
	colEpochEnd    = 25  //
	colMStart      = 26  //  27-35   mean anomaly, degrees
	colMEnd        = 35  //
	colPeriStart   = 37  //  38-46   argument of perihelion, degrees, J2000
	colPeriEnd     = 46  //
	colNodeStart   = 48  //  49-57   longitude of ascending node, degrees, J2000
	colNodeEnd     = 57  //
	colInclStart   = 59  //  60-68   inclination, degrees, J2000
	colInclEnd     = 68  //
	colEccStart    = 70  //  71-79   eccentricity
	colEccEnd      = 79  //
	colAStart      = 92  //  93-103  semi-major axis, AU
	colAEnd        = 103 //
	colReadStart   = 166 // 167-194  readable designation
	colReadEnd     = 194 //
	colMinRowWidth = colReadEnd
)

// isElementRow separates a data row from the header MPCORB.DAT carries above
// it — a paragraph of prose, a blank line and a rule of dashes, none of which
// the subset files have.
//
// The test is structural rather than a line count or a "skip until dashes"
// scan: a row is a row if it is wide enough to reach the readable designation
// and its eccentricity field parses. Header prose does neither, and a format
// change that moved the columns would fail loudly here rather than yielding
// plausible garbage.
func isElementRow(row string) bool {
	if len(row) < colMinRowWidth {
		return false
	}

	_, err := strconv.ParseFloat(strings.TrimSpace(row[colEccStart:colEccEnd]), 64)

	return err == nil
}

// parseRow converts one MPCORB row into a resolve.Target.
func parseRow(row string) (resolve.Target, error) {
	t := resolve.Target{
		ID:          strings.TrimSpace(row[:colDesigEnd]),
		Name:        strings.TrimSpace(row[colReadStart:colReadEnd]),
		Designation: strings.TrimSpace(row[colReadStart:colReadEnd]),
		Catalog:     "mpcorb",
		Kind:        resolve.KindAsteroid,
	}

	epoch, err := ParseEpoch(strings.TrimSpace(row[colEpochStart:colEpochEnd]))
	if err != nil {
		return resolve.Target{}, fmt.Errorf("%s: %w", t.ID, err)
	}

	for _, f := range []struct {
		name  string
		text  string
		set   func(float64)
		isDeg bool
	}{
		{"semi-major axis", row[colAStart:colAEnd], func(v float64) { t.SemiMajorAxis = v }, false},
		{"eccentricity", row[colEccStart:colEccEnd], func(v float64) { t.Eccentricity = v }, false},
		{"inclination", row[colInclStart:colInclEnd], func(v float64) { t.Inclination = angle.Deg(v) }, true},
		{"ascending node", row[colNodeStart:colNodeEnd], func(v float64) { t.AscendingNode = angle.Deg(v) }, true},
		{"argument of perihelion", row[colPeriStart:colPeriEnd], func(v float64) { t.ArgPeriapsis = angle.Deg(v) }, true},
		{"mean anomaly", row[colMStart:colMEnd], func(v float64) { t.MeanAnomaly = angle.Deg(v) }, true},
	} {
		v, err := strconv.ParseFloat(strings.TrimSpace(f.text), 64)
		if err != nil {
			return resolve.Target{}, fmt.Errorf("%w: %s: %s %q", ErrMalformedRow, t.ID, f.name, f.text)
		}

		f.set(v)
	}

	t.Epoch = epoch
	t.HasElements = true

	// H is absent from a handful of rows — one in Distant.txt — and G is
	// absent whenever H is. Left unset rather than defaulted: H = 0 is a
	// magnitude, and an object reported at absolute magnitude zero is the
	// brightest thing in the solar system rather than one nobody has measured.
	if h, err := strconv.ParseFloat(strings.TrimSpace(row[colHStart:colHEnd]), 64); err == nil {
		t.H = h
		t.HasH = true

		// G is only meaningful alongside H, and the MPC defaults it to 0.15
		// where it has not been fitted — which is a real published value, not
		// a gap, so it is taken as given.
		if g, err := strconv.ParseFloat(strings.TrimSpace(row[colGStart:colGEnd]), 64); err == nil {
			t.G = g
		}
	}

	return t, nil
}

// ParseEpoch decodes the MPC's five-character packed date — "K2669" is
// 2026 June 9 — into an epoch at 0h TT, which is the convention MPCORB's
// osculation epochs are given in.
//
// The packing is one character each for century, decade+year, month and day:
// I/J/K for 18xx/19xx/20xx, two digits of year, then a single character for
// month and for day where 1-9 are themselves and A onward continue the count
// (A = 10, so V = 31). It exists because the whole record is 202 fixed
// columns and a date could not have ten of them.
//
// Exported because the same packing appears throughout the MPC's files and a
// caller reading one of the others has the same problem to solve.
func ParseEpoch(packed string) (time.Time, error) {
	if len(packed) != epochWidth {
		return time.Time{}, fmt.Errorf("%w: %q is %d characters, want %d",
			ErrMalformedEpoch, packed, len(packed), epochWidth)
	}

	century, ok := map[byte]int{'I': 1800, 'J': 1900, 'K': 2000}[packed[0]]
	if !ok {
		return time.Time{}, fmt.Errorf("%w: %q has century marker %q, want I, J or K",
			ErrMalformedEpoch, packed, packed[0:1])
	}

	yy, err := strconv.Atoi(packed[1:3])
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %q has non-numeric year %q", ErrMalformedEpoch, packed, packed[1:3])
	}

	month, err := unpackDigit(packed[3])
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %q: month: %w", ErrMalformedEpoch, packed, err)
	}

	day, err := unpackDigit(packed[4])
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %q: day: %w", ErrMalformedEpoch, packed, err)
	}

	if month < 1 || month > 12 || day < 1 || day > 31 {
		return time.Time{}, fmt.Errorf("%w: %q decodes to month %d day %d", ErrMalformedEpoch, packed, month, day)
	}

	// The Julian Date of a calendar midnight is the same number whichever
	// scale labels it; what differs is which instant that number names. So
	// the date is turned into a JD through the UTC constructor and then
	// labelled TT, which is the scale MPCORB's epochs are actually given in.
	// Reading it back as UTC instead would be wrong by ΔT — 69 s today, and
	// a minute is not nothing when the elements are osculating.
	midnight := time.Date(century+yy, time.Month(month), day, 0, 0, 0, 0, time.LocationUTC)

	return time.FromJD(midnight.JD(), time.TT), nil
}

// epochWidth is the packed date's fixed width in the MPCORB record.
const epochWidth = 5

// unpackDigit reads one character of the MPC's extended count: '1' through
// '9' are themselves, and 'A' onward continue from 10, so 'V' is 31.
//
// '0' is rejected here rather than left to the range check on the decoded
// date. Both refuse it, so the difference is only in what the caller is told —
// but "0 is not a month character" points at the byte to look at, and
// "decodes to month 0" points at a date that was never in the file.
//
// The error is plain rather than wrapped: ParseEpoch attaches
// ErrMalformedEpoch once, so a caller sees one sentinel and one message
// instead of the same sentinel nested inside itself.
func unpackDigit(c byte) (int, error) {
	switch {
	case c >= '1' && c <= '9':
		return int(c - '0'), nil
	case c >= 'A' && c <= 'Z':
		return int(c-'A') + 10, nil
	default:
		return 0, fmt.Errorf("%q is not 1-9 or A-Z", string(c)) //nolint:err113 // wrapped by ParseEpoch, which owns the sentinel
	}
}

// clientOf is the client opts selected, or the process default.
//
// [resolve.ClientOf] returns nil for "none", which is what [api.WithRemote]
// wants; this path calls GetFile directly and needs a receiver.
func clientOf(opts []resolve.Option) *remote.Client {
	if c := resolve.ClientOf(opts); c != nil {
		return c
	}

	return remote.Default()
}
