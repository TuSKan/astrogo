---
type: Fixed
pr: 966
---
**`remote.GetFile` checks download consent before any request.** It used to ask a mutable endpoint's source whether its cached copy was current first, so an EOP lookup past the bulletin, with no consent given, blocked 17 s on an unreachable IERS server. A caller without consent is now served what is cached, as offline mode serves it, and refused only when nothing is.
