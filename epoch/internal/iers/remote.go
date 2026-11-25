package iers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/TuSKan/astrogo/remote"
)

// cacheName is the name finals2000A.all is cached under.
const cacheName = "finals2000A.data"

// remoteLoader is the [Loader] the lazy load uses unless another is
// registered: the IERS bulletin through remote, so its offline mode, download
// policy, endpoint overrides and cache apply to it as to every other file.
type remoteLoader struct{}

// Cached returns the finals2000A file already in the cache directory.
//
// Deliberately not remote.GetFile: that path requires a recorded Signature/ETag,
// which a hand-pre-seeded file never has. Reading the cache object
// directly is what finds a file copied in by hand for an offline or
// air-gapped deployment.
//
// No bulletin is [ErrNoEOPData], an ordinary state. One that is there and
// cannot be read is returned as itself, so it is not mistaken for one never
// seeded: only the second is the operator's to fix by seeding it (#509).
func (remoteLoader) Cached(ctx context.Context) (Data, error) {
	fsys, prefix, err := remote.CacheDir(ctx, remote.IERSFinals2000A)
	if err != nil {
		return Data{}, ErrNoEOPData
	}

	key := prefix + cacheName

	info, err := fs.Stat(fsys, key)
	if errors.Is(err, fs.ErrNotExist) {
		return Data{}, ErrNoEOPData
	}

	if err != nil {
		return Data{}, fmt.Errorf("iers: stat %s: %w", key, err)
	}

	raw, err := fs.ReadFile(fsys, key)
	if err != nil {
		return Data{}, fmt.Errorf("iers: read %s: %w", key, err)
	}

	return Data{Raw: raw, ModTime: info.ModTime()}, nil
}

// Fetch downloads finals2000A.all, subject to the download policy.
func (remoteLoader) Fetch(ctx context.Context) (Data, error) {
	// The network step, and offline mode and the policy decide it before
	// anything else does. Either way remote.GetFile would serve a cached
	// bulletin instead (#964), which is the one Cached has just read and found
	// wanting, and the lazy load would record it as fetched from the network.
	if _, err := remote.URL(remote.IERSFinals2000A); err != nil {
		return Data{}, fmt.Errorf("iers: fetch EOP data: %w", err)
	}

	ep, _ := remote.Lookup(remote.IERSFinals2000A)
	if err := remote.CheckDownload(remote.IERSFinals2000A, "finals2000A.all", ep.ApproxSize); err != nil {
		return Data{}, fmt.Errorf("iers: fetch EOP data: %w", err)
	}

	// remote.GetFile reuses the cache untouched when the source's current ETag
	// shows the IERS bulletin has not changed since it was last
	// downloaded — a content check rather than a wall-clock expiry, since
	// finals2000A is updated on IERS's schedule, not ours.
	//
	// The bulletin is parsed through remote.WithRead, on whichever copy
	// GetFile returns. A corrupt download is never trusted as the new cache,
	// and a cached copy that no longer parses is replaced in the same call.
	// With WithValidate, which sees only a download, such a copy came back
	// from here for as long as the source's ETag held, and the bulletin failed
	// to parse each time (#1043).
	var raw []byte

	read := func(fsys remote.FS, key string) error {
		b, err := fs.ReadFile(fsys, key)
		if err != nil {
			return fmt.Errorf("iers: read EOP data: %w", err)
		}

		if _, perr := ParseFinals2000A(bytes.NewReader(b)); perr != nil {
			return fmt.Errorf("iers: EOP data does not parse: %w", perr)
		}

		raw = b

		return nil
	}

	if _, _, err := remote.GetFile(ctx, remote.IERSFinals2000A, "finals2000A.all",
		remote.WithCacheName(cacheName), remote.WithRead(read)); err != nil {
		return Data{}, fmt.Errorf("iers: fetch EOP data: %w", err)
	}

	return Data{Raw: raw}, nil
}
