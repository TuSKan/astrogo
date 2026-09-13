---
type: Security
pr: 293
---
**SFTP dependency security update.** Upgrade `golang.org/x/crypto` to v0.56.0,
fixing SSH channel deadlock denial-of-service vulnerabilities GO-2026-6354 and
GO-2026-6355 reachable through the SFTP blob driver.
