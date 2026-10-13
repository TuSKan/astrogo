---
type: Fixed
pr: 725
---
**`magnitude.PhaseSphere` is documented as the illuminated fraction it computes**, the law satellite standard magnitudes are defined with, not as a Lambertian sphere. A Lambertian sphere dims 0.49 mag more at 90° phase; its law is the one `PhaseCylinder` computes. The arithmetic is unchanged.
