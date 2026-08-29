---
type: Fixed
pr: 63
---
**The Horizons corpus sampled epochs whose reference values were still moving.** Its regular span ran to 2027-01-01, so the last epoch depended on *predicted* Earth orientation and shifted by up to 8.4e-05 degrees after generation — `TestGenerateCorpus` then reported a dirty diff on every run, for a corpus nobody had touched. The span now ends well inside the settled past, and `TestCorpusEpochsAreSettled` enforces it. 255 entries become 300, all with final Earth orientation.
