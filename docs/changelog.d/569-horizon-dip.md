---
type: Fixed
pr: 569
---
**The horizon dip is documented as what it is.** `Site.HorizonDip` returns the apparent dip, 1.76′√h with terrestrial refraction (0.82° at 786 m), but its doc called it geometric and quoted the geometric 0.90°. Seven other places used the same label, and `VisibilityEvents` claimed 34′ of refraction its threshold does not add. A test now pins the dip to its doc.
