//go:build !windows

package testutil

// platformErrnos needs nothing beyond the POSIX numbers Unreachable already
// checks; see the Windows file for the platform that does.
var platformErrnos []error
