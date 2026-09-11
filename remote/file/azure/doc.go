// Package azure registers the "azblob://" scheme. Blank-import it from a build
// that opens an azblob:// endpoint:
//
//	import _ "github.com/TuSKan/astrogo/remote/file/azure"
//
// It is a separate package purely so the Azure SDK stays out of every build
// that doesn't talk to Azure Blob Storage; it exports nothing and has no
// init() of its own beyond the driver's.
//
// The URL's host is the container name, and the rest rides in its query
// string, which gocloud.dev/blob/azureblob's URL opener parses: domain,
// protocol, cdn, localemu, documented at
// https://pkg.go.dev/gocloud.dev/blob/azureblob. So a container is addressed
// entirely through remote.SetURL or remote.SetDataDir — this package needs no
// configuration API, and remote itself needs no Azure-specific knowledge.
//
// The account and its credentials are the exception, and they come from the
// environment rather than the URL: AZURE_STORAGE_ACCOUNT names the account,
// and one of AZURE_STORAGE_KEY, AZURE_STORAGE_CONNECTION_STRING,
// AZURE_STORAGE_SAS_TOKEN or the azidentity default chain authenticates it.
// That is the driver's design, not astrogo's, and it is worth knowing because
// an azblob:// URL is therefore not self-contained the way an s3:// or sftp://
// one is: the same URL reaches a different account under a different
// environment.
package azure

import _ "gocloud.dev/blob/azureblob" // registers the "azblob" scheme with blob.DefaultURLMux
