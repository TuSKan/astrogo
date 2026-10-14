---
type: Fixed
pr: 775
---
**`fits` reads and writes ASCII tables as FITS 4.0 §7.2 defines them.** TNULLn was ignored, so a declared null like `*****` failed the whole read and `-999.00` read as a value; a blank integer field is now 0, as the standard says, rather than null; a real field without a decimal point takes the implied one (`12345` in `F8.2` is 123.45); and `1.5-3` reads as 1.5e-3. The writer now declares an empty TNULLn for every column holding a null, which a standard reader would otherwise read as 0.
