---
type: Fixed
pr: 1283
---
**`fits.Write` refuses an image whose axes declare pixels it holds no tensor for**, with `fits.ErrNotWritable`, where it wrote a header describing data the file did not carry.
