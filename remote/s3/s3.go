package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	gofs "github.com/ungerik/go-fs"
	s3fs "github.com/ungerik/go-fs/s3fs"

	"github.com/TuSKan/astrogo/remote"
)

// defaultRegion is used for SigV4 signing when neither WithRegion nor the
// loaded AWS config supplies one. Copernicus Data Space Ecosystem's own
// documentation uses this literal value — its S3-compatible endpoint
// does not have real per-bucket regional semantics the way AWS S3 does,
// but SigV4 still requires a non-empty region string.
const defaultRegion = "default"

// registerConfig carries per-Register options.
type registerConfig struct {
	client *awss3.Client
	region string
}

// Option customizes a single Register call.
type Option func(*registerConfig)

// WithClient injects a pre-built *s3.Client instead of constructing one
// from the AWS SDK v2 default credential chain — the only way to test
// this package offline, e.g. against an httptest.Server or a local
// S3-compatible service with static credentials.
func WithClient(c *awss3.Client) Option {
	return func(cfg *registerConfig) { cfg.client = c }
}

// WithRegion overrides the SigV4 signing region used when no client is
// injected. Defaults to the loaded AWS config's own Region, falling back
// to defaultRegion if that's empty too.
func WithRegion(region string) Option {
	return func(cfg *registerConfig) { cfg.region = region }
}

// bucketState is the per-endpoint state Register populates.
type bucketState struct {
	// client is used for exactly one thing: HeadObject metadata calls
	// (Probe's ETag/size, and FetchInto's pre-download exact-size
	// consent check). It never touches an object's body — see FetchInto's
	// doc comment for why that goes through s3fs/gofs.File instead, and
	// this type's own doc comment for why HeadObject specifically cannot.
	client *awss3.Client
	bucket string
	// fs is the gofs.FileSystem this Register call installed into go-fs's
	// own global filesystem registry (see s3fsRegistry's doc comment for
	// why it must be tracked and explicitly Unregistered on replacement,
	// rather than left to a plain re-Register).
	fs gofs.FileSystem
}

var (
	stateMu sync.RWMutex
	state   = map[remote.EndpointID]*bucketState{}
)

// Register installs the S3 transport for remote.KindS3 and prepares the
// connection backing endpoint id's bucket. Call it once, explicitly,
// before the first remote.GetFile against a KindS3 endpoint; without it
// GetFile fails with remote.ErrNoTransport. Calling Register again for
// the same id replaces its state; calling it for a different id adds a
// second entry — both share the one remote.RegisterTransport call, which
// is itself idempotent (last registration wins, and here every
// registration installs the same transport{} value regardless of id).
//
// See this package's doc comment for the credential contract (AWS SDK v2
// default chain only) and why FetchInto/Probe are split the way they are
// between s3fs and a direct HeadObject call.
func Register(ctx context.Context, id remote.EndpointID, opts ...Option) error {
	ep, ok := remote.Lookup(id)
	if !ok {
		return fmt.Errorf("%w: %q", remote.ErrUnknownEndpoint, id)
	}

	if ep.Kind != remote.KindS3 {
		return fmt.Errorf("%w: %q", ErrNotS3Endpoint, id)
	}

	if ep.Bucket == "" {
		return fmt.Errorf("%w: %q", ErrNoBucket, id)
	}

	// remote.URL gates offline mode / Disable / SetURL overrides the same
	// way every other endpoint access in this codebase does, before any
	// client is built.
	baseURL, err := remote.URL(id)
	if err != nil {
		return fmt.Errorf("remote/s3: register %q: %w", id, err)
	}

	var cfg registerConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	client := cfg.client
	if client == nil {
		awsCfg, err := config.LoadDefaultConfig(ctx)
		if err != nil {
			return fmt.Errorf("remote/s3: load AWS credentials: %w", err)
		}

		region := cfg.region
		if region == "" {
			region = awsCfg.Region
		}

		if region == "" {
			region = defaultRegion
		}

		client = awss3.New(awss3.Options{
			Region:       region,
			Credentials:  awsCfg.Credentials,
			BaseEndpoint: aws.String(baseURL),
			UsePathStyle: true,
		})
	}

	stateMu.Lock()
	defer stateMu.Unlock()

	// go-fs's own Register (github.com/ungerik/go-fs/registry.go) is
	// refcounted BY PREFIX, not replace-on-reregister — confirmed by
	// reading it directly: a second Register call for an already-present
	// prefix ("s3://"+ep.Bucket) only increments a counter and returns
	// early; it never overwrites the stored *fs.FileSystem*. So without
	// this Unregister, re-Registering this id (rotated credentials, a
	// new injected WithClient in a test, ...) would silently leave every
	// gofs.File("s3://"+ep.Bucket+"...") — including FetchInto/Probe's
	// own s3URIFor calls — resolving through the FIRST registration's
	// client forever, no matter how many times Register is called again.
	if prev, ok := state[id]; ok && prev.fs != nil {
		gofs.Unregister(prev.fs)
	}

	// Registers gofs.File("s3://"+ep.Bucket+"...") through go-fs's own
	// filesystem registry — this is what FetchInto's OpenReader call
	// below actually goes through, not a side effect for someone else.
	newFS := s3fs.NewAndRegister(client, ep.Bucket, true)
	state[id] = &bucketState{client: client, bucket: ep.Bucket, fs: newFS}

	remote.RegisterTransport(remote.KindS3, transport{})

	return nil
}

func lookupState(id remote.EndpointID) (*bucketState, error) {
	stateMu.RLock()
	defer stateMu.RUnlock()

	st, ok := state[id]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrNotRegistered, id)
	}

	return st, nil
}

// transport implements remote.Transport for remote.KindS3 endpoints.
type transport struct{}

var _ remote.Transport = transport{}

// s3URIFor builds the gofs.File URI that s3fs's own CleanPathFromURI
// (strings.TrimPrefix(uri, "s3://"+bucket), confirmed by reading
// s3fs.go directly — no separator handling at all) turns into exactly
// key, with no leading slash. s3fs's own "natural" URI form
// ("s3://bucket/key") would instead trim to "/key" — a leading slash —
// which every s3fs method then uses verbatim as the literal S3 object
// Key. That's s3fs's own self-consistent convention for objects it
// wrote itself, but real CAMS EODATA objects (written by Copernicus's
// own pipeline, confirmed against the user's own working `aws s3 cp`
// command) have no leading slash. Concatenating bucket and key with no
// separating slash compensates for that trim rather than tripping over
// it: TrimPrefix("s3://"+bucket+key, "s3://"+bucket) == key exactly.
func s3URIFor(bucket, key string) gofs.File {
	return gofs.File("s3://" + bucket + key)
}

// FetchInto downloads endpoint id's object at key into dest via
// gofs.File.OpenReader() — i.e. through s3fs, not a direct GetObject
// call — matching the rest of this codebase's rule that remote (and its
// transports) read file content through go-fs's own methods rather than
// reinventing them. One direct HeadObject call still runs first: s3fs's
// own File.OpenReader fetches eagerly (HEAD then GET as one call with no
// way to intercept in between — confirmed by reading its
// implementation), so without a HeadObject beforehand a SizeVaries
// endpoint like CopernicusEODATA would have no real exact-size consent
// gate before real network transfer begins. That HeadObject is cheap
// (metadata only, no body) and is the same call Probe already needs for
// the ETag s3fs's own Stat cannot provide (see bucketState's doc
// comment) — not a second, separate design.
//
// s3fs@v0.1.0's OpenReader fully buffers the object in memory before
// returning (confirmed by reading its source — every code path ends in
// io.ReadAll or a full in-memory manager.WriteAtBuffer; there is no
// streaming path in this dependency version at all). Routing through it
// therefore does not bound peak memory below the object's own size —
// documented here plainly, not hidden — but it does mean this package
// no longer hand-rolls a second GetObject/body-streaming implementation
// alongside s3fs's own.
func (transport) FetchInto(ctx context.Context, id remote.EndpointID, key string, dest gofs.File, timeout time.Duration, validate func([]byte) error, progress func(downloaded, total int64)) error {
	st, err := lookupState(id)
	if err != nil {
		return err
	}

	ep, _ := remote.Lookup(id)

	if err := remote.CheckDownload(id, key, ep.ApproxSize); err != nil {
		return fmt.Errorf("remote/s3: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	total, err := headObjectSize(ctx, st, key)
	if err != nil {
		return err
	}

	if err := remote.CheckDownload(id, key, total); err != nil {
		return fmt.Errorf("remote/s3: %w", err)
	}

	if ep.ApproxSize == remote.SizeVaries {
		log.Printf("remote/s3: downloading %s (endpoint %s, %d bytes)", dest, id, total)
	} else {
		log.Printf("remote/s3: downloading %s (endpoint %s, approx %d bytes)", dest, id, ep.ApproxSize)
	}

	r, err := s3URIFor(st.bucket, key).OpenReader()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %s: no such key", remote.ErrDownloadFailed, key)
		}

		return fmt.Errorf("%w: %s: %w", remote.ErrDownloadFailed, key, err)
	}

	defer func() { _ = r.Close() }()

	var body io.Reader = r
	if progress != nil {
		body = &progressReader{r: r, total: total, onProgress: progress}
	}

	if validate != nil {
		data, err := io.ReadAll(body)
		if err != nil {
			return fmt.Errorf("%w: %s: %w", remote.ErrDownloadFailed, key, err)
		}

		if verr := validate(data); verr != nil {
			return fmt.Errorf("remote/s3: validate %s: %w", key, verr)
		}

		if err := remote.Save(bytes.NewReader(data), dest); err != nil {
			return fmt.Errorf("%w: %w", remote.ErrDownloadFailed, err)
		}

		return nil
	}

	if err := remote.Save(body, dest); err != nil {
		return fmt.Errorf("%w: %w", remote.ErrDownloadFailed, err)
	}

	return nil
}

// Probe returns endpoint id's object at key's current remote.Signature via
// a direct HeadObject call — not s3fs.Stat, which discards the ETag
// remote.Signature.ETag needs (confirmed by reading s3fs's Stat
// implementation: it builds a private fileInfo{name,size,time} and never
// reads the HeadObject response's ETag field at all — Sys() also
// unconditionally returns nil, so there is no escape hatch to recover it
// through go-fs's own File.Stat() either).
func (transport) Probe(ctx context.Context, id remote.EndpointID, key string) (remote.Signature, error) {
	st, err := lookupState(id)
	if err != nil {
		return remote.Signature{}, err
	}

	head, err := st.client.HeadObject(ctx, &awss3.HeadObjectInput{
		Bucket: aws.String(st.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var notFound *types.NotFound
		if errors.As(err, &notFound) {
			return remote.Signature{}, &remote.HTTPError{StatusCode: 404, Body: key}
		}

		return remote.Signature{}, fmt.Errorf("remote/s3: head %s: %w", key, err)
	}

	return remote.Signature{ETag: aws.ToString(head.ETag), ContentLength: aws.ToInt64(head.ContentLength)}, nil
}

// headObjectSize is FetchInto's pre-download exact-size check; see
// FetchInto's own doc comment for why this direct HeadObject call is
// necessary ahead of s3fs.File.OpenReader's own eager fetch.
func headObjectSize(ctx context.Context, st *bucketState, key string) (int64, error) {
	head, err := st.client.HeadObject(ctx, &awss3.HeadObjectInput{
		Bucket: aws.String(st.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var notFound *types.NotFound
		if errors.As(err, &notFound) {
			return 0, fmt.Errorf("%w: %s: no such key", remote.ErrDownloadFailed, key)
		}

		return 0, fmt.Errorf("%w: %s: %w", remote.ErrDownloadFailed, key, err)
	}

	return aws.ToInt64(head.ContentLength), nil
}

// progressReader wraps r, invoking onProgress after every Read that
// returns data with the running byte count and total (0 if unknown).
// Mirrors remote's own unexported progressReader (remote/file.go) — kept
// as a small local copy since that type isn't exported, and this
// package's whole reason to exist is to avoid remote depending on
// anything S3-specific.
type progressReader struct {
	r          io.Reader
	total      int64
	read       int64
	onProgress func(downloaded, total int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.read += int64(n)
		p.onProgress(p.read, p.total)
	}

	//nolint:wrapcheck // must forward the underlying error (incl. io.EOF) unwrapped: io.Copy/io.ReadAll identity-check it via errors.Is
	return n, err
}
