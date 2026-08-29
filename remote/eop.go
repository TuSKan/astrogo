package remote

import (
	"context"
	"fmt"
	"io"

	atime "github.com/TuSKan/astrogo/time"
)

// eopLoader supplies IERS finals2000A data to astrogo/time.
//
// The dependency runs this way round on purpose. astrogo/time used to
// import this package to fetch EOP data, which meant that computing a
// Julian date linked gocloud, gRPC and protobuf — about 17 MB of binary
// for arithmetic that touches none of it. Inverting it costs nothing:
// download consent can only be granted through [EnableDownloads], so any
// program that could fetch EOP data already imports this package and so
// still gets a loader registered. One that imports neither degrades to
// zero EOP, as an unconsented program always has.
type eopLoader struct{}

// Cached returns the finals2000A file already in the cache directory.
//
// Deliberately not GetFile: that path requires a recorded Signature/ETag,
// which a hand-pre-seeded file never has. Reading the cache object
// directly is what finds a file copied in by hand for an offline or
// air-gapped deployment.
func (eopLoader) Cached(ctx context.Context) (atime.EOPData, error) {
	bucket, prefix, err := CacheDir(ctx, IERSFinals2000A)
	if err != nil {
		return atime.EOPData{}, atime.ErrNoEOPData
	}

	key := prefix + eopCacheName

	attrs, err := bucket.Attributes(ctx, key)
	if err != nil {
		return atime.EOPData{}, atime.ErrNoEOPData
	}

	raw, err := bucket.ReadAll(ctx, key)
	if err != nil {
		return atime.EOPData{}, atime.ErrNoEOPData
	}

	return atime.EOPData{Raw: raw, ModTime: attrs.ModTime}, nil
}

// Fetch downloads finals2000A.all, subject to download consent.
func (eopLoader) Fetch(ctx context.Context) (atime.EOPData, error) {
	// GetFile reuses the cache untouched when the source's current ETag
	// shows the IERS bulletin has not changed since it was last
	// downloaded — a content check rather than a wall-clock expiry, since
	// finals2000A is updated on IERS's schedule, not ours. WithValidate
	// parses a fresh download before it is cached, so a corrupt response
	// is never trusted as the new cache.
	bucket, key, err := GetFile(ctx, IERSFinals2000A, "finals2000A.all",
		WithCacheName(eopCacheName),
		WithValidate(func(r io.Reader) error {
			if _, perr := atime.ParseFinals2000A(r); perr != nil {
				return fmt.Errorf("remote: EOP data does not parse: %w", perr)
			}

			return nil
		}))
	if err != nil {
		return atime.EOPData{}, fmt.Errorf("remote: fetch EOP data: %w", err)
	}

	raw, err := bucket.ReadAll(ctx, key)
	if err != nil {
		return atime.EOPData{}, fmt.Errorf("remote: read EOP data: %w", err)
	}

	return atime.EOPData{Raw: raw}, nil
}

// eopCacheName is the name finals2000A.all is cached under.
const eopCacheName = "finals2000A.data"

//nolint:gochecknoinits // the registration this package exists to provide
func init() { atime.RegisterEOPLoader(eopLoader{}) }
