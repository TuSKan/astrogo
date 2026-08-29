---
type: Added
pr: 71
---
**`constants.DE440` — the JPL ephemeris mass parameters**, the first of the five pieces roadmap item 39 needs for central-body propagation of planetary moons. IAU 2015 B3 publishes nominal mass parameters for only the Sun, Earth and Jupiter, so anything needing Mars or Uranus had nowhere to look. Each planet appears twice, system and body: using one where the other belongs is a silent error the size of the satellite system, which for Pluto and Charon is 12%. Every value is checked against NAIF's own kernel by a network test rather than trusted to transcription. Adds `remote.NAIFPCK`.
