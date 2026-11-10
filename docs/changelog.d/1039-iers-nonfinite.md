---
type: Fixed
pr: 1039
---
**`time.ParseFinals2000A` refuses an IERS bulletin holding a NaN or infinite field**, naming the line. A NaN MJD in the bulletin's last row used to make every UT1 conversion past the end of the data panic, and other non-finite fields came back as NaN or infinite Earth orientation with no error.
