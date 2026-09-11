// Package gcs registers the "gs://" scheme. Blank-import it from a build that
// opens a gs:// endpoint:
//
//	import _ "github.com/TuSKan/astrogo/remote/file/gcs"
//
// It is a separate package purely so the Google Cloud client libraries stay
// out of every build that doesn't talk to GCS; it exports nothing and has no
// init() of its own beyond the driver's.
//
// The URL's host is the bucket name, and the rest rides in its query string,
// which gocloud.dev/blob/gcsblob's URL opener parses: anonymous, access_id,
// private_key_path, universe_domain, documented at
// https://pkg.go.dev/gocloud.dev/blob/gcsblob. So a bucket is addressed
// entirely through remote.SetURL or remote.SetDataDir — this package needs no
// configuration API, and remote itself needs no GCS-specific knowledge.
// Credentials resolve through Google's Application Default Credentials
// (GOOGLE_APPLICATION_CREDENTIALS, gcloud's own login, or the metadata server
// on GCE); astrogo reads no credential file of its own. A public bucket needs
// "?anonymous=true", since ADC failing is otherwise an error rather than a
// fallback.
package gcs

import _ "gocloud.dev/blob/gcsblob" // registers the "gs" scheme with blob.DefaultURLMux
