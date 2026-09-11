// Package sftp registers the "sftp://" scheme. Blank-import it from a build
// that opens an sftp:// endpoint:
//
//	import _ "github.com/TuSKan/astrogo/remote/file/sftp"
//
// It is a separate package purely so the SSH and SFTP libraries stay out of
// every build that doesn't talk to an SFTP server; it exports nothing and has
// no init() of its own beyond the driver's.
//
// The URL names the server, the login and the bucket root —
// "sftp://user@host:2222/a/directory" — and everything else rides in its query
// string, which github.com/TuSKan/gocloud-ext/blob/sftpblob's URL opener
// parses: private_key_path, private_key_env, private_key_passphrase_env,
// known_hosts_path, insecure_skip_verify, create_dir, metadata, timeout. So a
// server is addressed entirely through remote.SetURL or remote.SetDataDir —
// this package needs no configuration API, and remote itself needs no
// SFTP-specific knowledge.
//
// Authentication is tried in one order: a private key if private_key_path or
// private_key_env names one, then a password from the URL's userinfo, then the
// agent at SSH_AUTH_SOCK. Host keys are verified against ~/.ssh/known_hosts
// unless known_hosts_path says otherwise, and the driver rejects an
// unrecognised or malformed query parameter rather than ignoring it — a
// misspelled known_hosts_path would otherwise fall back to the default file
// silently, which is a security setting failing open.
package sftp

import _ "github.com/TuSKan/gocloud-ext/blob/sftpblob" // registers the "sftp" scheme with blob.DefaultURLMux
