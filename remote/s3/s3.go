package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
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
	client *awss3.Client
	bucket string
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
// default chain only) and why FetchInto/Probe bypass s3fs's own
// File-reading methods.
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
	state[id] = &bucketState{client: client, bucket: ep.Bucket}
	stateMu.Unlock()

	// Registers gofs.File("s3://"+ep.Bucket+"/...") as a working value
	// through go-fs's own filesystem registry for any *other* future
	// caller in this codebase — not used by this package's own
	// FetchInto/Probe below; see doc.go for why.
	s3fs.NewAndRegister(client, ep.Bucket, true)

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

// FetchInto downloads endpoint id's object at key (the raw, unprefixed S3
// key — no leading slash, never built from a gofs.File URI; see doc.go)
// into dest, streaming the SDK's GetObjectOutput.Body directly through to
// remote.Save so peak memory stays bounded by the copy buffer, not the
// object size, in the common (non-validated) case. With validate
// non-nil, the body is buffered and checked before being written, the
// same tradeoff remote's built-in HTTP transport already accepts for
// WithValidate.
func (transport) FetchInto(ctx context.Context, id remote.EndpointID, key string, dest gofs.File, timeout time.Duration, validate func([]byte) error, progress func(downloaded, total int64)) error {
	st, err := lookupState(id)
	if err != nil {
		return err
	}

	ep, _ := remote.Lookup(id)

	if err := remote.CheckDownload(id, key, ep.ApproxSize); err != nil {
		return fmt.Errorf("remote/s3: %w", err)
	}

	if ep.ApproxSize == remote.SizeVaries {
		log.Printf("remote/s3: downloading %s (endpoint %s, size varies)", dest, id)
	} else {
		log.Printf("remote/s3: downloading %s (endpoint %s, approx %d bytes)", dest, id, ep.ApproxSize)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	out, err := st.client.GetObject(ctx, &awss3.GetObjectInput{
		Bucket: aws.String(st.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var noSuchKey *types.NoSuchKey
		if errors.As(err, &noSuchKey) {
			return fmt.Errorf("%w: %s: no such key", remote.ErrDownloadFailed, key)
		}

		return fmt.Errorf("%w: %s: %w", remote.ErrDownloadFailed, key, err)
	}

	defer func() { _ = out.Body.Close() }()

	total := aws.ToInt64(out.ContentLength)
	if err := remote.CheckDownload(id, key, total); err != nil {
		return fmt.Errorf("remote/s3: %w", err)
	}

	var body io.Reader = out.Body

	if progress != nil {
		body = &progressReader{r: out.Body, total: max(total, 0), onProgress: progress}
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
// reads the HeadObject response's ETag field at all).
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
