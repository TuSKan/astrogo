---
type: Fixed
pr: 738
---
**Satellites catalogued since 2026-07-11 get TLEs that name them.** CelesTrak ran out of five-digit catalog numbers that day, and `catalog/norad`'s `GP.ToTLE` clamped every six-digit one to 99999. It now writes Alpha-5 up to 339999 and returns no lines beyond, and `sgp4.ParseTLE` reads Alpha-5 back.
