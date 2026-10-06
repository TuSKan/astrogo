//go:build windows

package testutil

import "syscall"

// platformErrnos are Winsock's numbers for a connection the network dropped.
//
// Go's syscall.ECONNRESET is an invented value on Windows that errors.Is
// never matches against a real socket error: measured, a connection the
// server reset arrived as WSAECONNRESET and fell through every case in
// Unreachable, while the same reset on Linux or macOS was counted (#505).
// A refused dial, an unreachable host and a timeout need nothing here: the
// dial and timeout checks catch them on every platform.
var platformErrnos = []error{
	syscall.WSAECONNRESET,
	syscall.WSAECONNABORTED,
}
