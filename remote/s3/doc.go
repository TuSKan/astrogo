// Package s3 registers the "s3://" scheme for remote/file. Blank-import it
// from a build that opens an s3:// endpoint:
//
//	import _ "github.com/TuSKan/astrogo/remote/s3"
//
// It is a separate package purely so the AWS SDK stays out of every build
// that doesn't talk to S3; it exports nothing and has no init() of its own
// beyond the driver's.
//
// Connection details ride in the endpoint URL's query string, which
// gocloud.dev/blob/s3blob's URL opener parses: region, endpoint,
// hostname_immutable, use_path_style, and the rest documented at
// https://pkg.go.dev/gocloud.dev/blob/s3blob. So a non-AWS S3-compatible
// service is addressed entirely through remote.SetURL — this package needs
// no configuration API, and remote itself needs no S3-specific knowledge.
// Credentials resolve through the AWS SDK v2 default chain (environment,
// ~/.aws/credentials, IAM role); astrogo reads no credential file of its
// own.
package s3

import _ "gocloud.dev/blob/s3blob" // registers the "s3" scheme with blob.DefaultURLMux
