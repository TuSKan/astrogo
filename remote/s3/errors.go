package s3

import "errors"

// Sentinel errors returned by this package. Match with errors.Is.
var (
	// ErrNotRegistered is returned by FetchInto/Probe when Register was
	// never called for the endpoint being fetched.
	ErrNotRegistered = errors.New("remote/s3: endpoint not registered — call s3.Register first")

	// ErrNotS3Endpoint is returned by Register when id does not name a
	// remote.KindS3 endpoint.
	ErrNotS3Endpoint = errors.New("remote/s3: endpoint is not a KindS3 endpoint")

	// ErrNoBucket is returned by Register when id's Endpoint.Bucket is
	// empty.
	ErrNoBucket = errors.New("remote/s3: endpoint has no Bucket configured")
)
