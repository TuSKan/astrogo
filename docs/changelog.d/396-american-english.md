---
type: Changed
pr: 396
---
**American English throughout.** Doc comments, test names, and the text of a
few error and log messages now spell "meter", "center", "color", "behavior",
"labeled" and the -ize forms, matching the exported names #364 and #383
already renamed. Sentinel errors are unchanged, so `errors.Is` still matches;
only the message text differs (#382, #384, #385, #386, #387, #356).
