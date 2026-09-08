---
type: Fixed
pr: 228
---
**A kernel that could not be read was deleted as corrupt.** "Access is denied"
and a checksum mismatch shared one branch, so a process losing a race to a file
lock deleted the shared 32 MB kernel out from under every other process — the
observed cause of intermittent Windows CI failures. Only a proven content
failure is destructive now; an I/O failure leaves the file for the next open to
retry (#227).
