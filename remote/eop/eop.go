// Package eop supplies IERS Earth Orientation Parameters to astrogo/time.
// Blank-import it from a program whose accuracy depends on them:
//
//	import _ "github.com/TuSKan/astrogo/remote/eop"
//
// Without it, time.Time.EOP, .UTC and .UT1 report zero DUT1 and polar
// motion and log one warning saying so — costing roughly an arcsecond of
// topocentric position and up to 0.9 s of UT1.
//
// # Why it is a separate package
//
// The dependency between time and remote runs backwards here, and has to.
// astrogo/time used to import remote to fetch EOP data, which meant that
// computing a Julian date linked gocloud, gRPC and protobuf — about 17 MB
// of binary for arithmetic that touches none of it. So remote registers a
// loader into time rather than time reaching for remote.
//
// That registration is an init(), and an init() has to belong to some
// package. It cannot be remote's own: this code needs remote.GetFile and
// remote.CacheDir, and a package cannot import the package that imports
// it. So it is here, one import below, and naming it is what turns it on.
//
// Which also makes it honest. IERS bulletins are somebody else's bandwidth
// on somebody else's schedule, and the rest of this module is built around
// never reaching for them unasked. A program that wants observatory-grade
// UT1 says so in one line, the same way it says remote.EnableDownloads.
//
// # What it does once imported
//
// time loads EOP data lazily, on the first Time.EOP, .UTC or .UT1 that
// needs it: a pre-seeded cache object is read unconditionally, and only
// the network step is gated behind
// remote.EnableDownloads(0, remote.IERSFinals2000A). Pre-seeding
// finals2000A.data into the cache is therefore the whole configuration an
// air-gapped deployment needs.
package eop

import (
	"context"
	"fmt"
	"io"

	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

// eopLoader is the time.EOPLoader this package registers. See the package
// doc comment for why it lives here rather than in remote.
type eopLoader struct{}

// Cached returns the finals2000A file already in the cache directory.
//
// Deliberately not remote.GetFile: that path requires a recorded Signature/ETag,
// which a hand-pre-seeded file never has. Reading the cache object
// directly is what finds a file copied in by hand for an offline or
// air-gapped deployment.
func (eopLoader) Cached(ctx context.Context) (time.EOPData, error) {
	bucket, prefix, err := remote.CacheDir(ctx, remote.IERSFinals2000A)
	if err != nil {
		return time.EOPData{}, time.ErrNoEOPData
	}

	key := prefix + eopCacheName

	attrs, err := bucket.Attributes(ctx, key)
	if err != nil {
		return time.EOPData{}, time.ErrNoEOPData
	}

	raw, err := bucket.ReadAll(ctx, key)
	if err != nil {
		return time.EOPData{}, time.ErrNoEOPData
	}

	return time.EOPData{Raw: raw, ModTime: attrs.ModTime}, nil
}

// Fetch downloads finals2000A.all, subject to download consent.
func (eopLoader) Fetch(ctx context.Context) (time.EOPData, error) {
	// remote.GetFile reuses the cache untouched when the source's current ETag
	// shows the IERS bulletin has not changed since it was last
	// downloaded — a content check rather than a wall-clock expiry, since
	// finals2000A is updated on IERS's schedule, not ours. remote.WithValidate
	// parses a fresh download before it is cached, so a corrupt response
	// is never trusted as the new cache.
	bucket, key, err := remote.GetFile(ctx, remote.IERSFinals2000A, "finals2000A.all",
		remote.WithCacheName(eopCacheName),
		remote.WithValidate(func(r io.Reader) error {
			if _, perr := time.ParseFinals2000A(r); perr != nil {
				return fmt.Errorf("remote: EOP data does not parse: %w", perr)
			}

			return nil
		}))
	if err != nil {
		return time.EOPData{}, fmt.Errorf("remote: fetch EOP data: %w", err)
	}

	raw, err := bucket.ReadAll(ctx, key)
	if err != nil {
		return time.EOPData{}, fmt.Errorf("remote: read EOP data: %w", err)
	}

	return time.EOPData{Raw: raw}, nil
}

// eopCacheName is the name finals2000A.all is cached under.
const eopCacheName = "finals2000A.data"

//nolint:gochecknoinits // the registration this package exists to provide
func init() { time.RegisterEOPLoader(eopLoader{}) }
