---
type: Fixed
pr: 370
---
**The last eight tests that skipped on an unidentified error now name the
condition they mean**, and the allowlist that exempted them is gone — each was
the same defect one level down. The symlink pair shows why no list: Windows
returns `ERROR_PRIVILEGE_NOT_HELD`, which does *not* satisfy
`errors.Is(err, fs.ErrPermission)`, so the natural predicate would have skipped
on every symlink failure while appearing to check for one. Two tests that
skipped when a loopback listener could not be bound are now fatal, including the
only test that exercises `Unreachable` against a socket the OS really refused.
