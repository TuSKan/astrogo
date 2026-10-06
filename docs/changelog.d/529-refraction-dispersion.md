---
type: Fixed
pr: 529
---
**`atmosphere.RefractionRigorous` dispersed light 16 times too little**, through
an unsourced 0.005/µm wavelength factor: 0.155″ between 0.40 and 0.70 µm at 30°
where SOFA gives 2.47″. `StandardRefraction` uses this model, so
`coord.Reducer.Disperse` inherited it. It now scales by the IAG (1999) dry-air
refractivity SOFA's `iauRefco` uses, and disperses like SOFA's model to 0.1%.
