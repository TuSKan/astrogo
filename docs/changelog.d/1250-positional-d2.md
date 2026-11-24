---
type: Changed — BREAKING
pr: 1250
---
**Positions are matched by their published errors, and the catalog has no built-in resolution.** Gaia and SIMBAD publish each record's position and proper-motion errors as `resolve.AstrometricError`. Two records that both publish them are one object when D² = Δᵀ(C₁+C₂)⁻¹Δ is at most 11.83, and `resolve.Association` records the D². A record that publishes none is matched by its identifiers alone, unless the caller sets `catalog.Resolver.Resolution` or `xmatch.WithResolution`. A record within reach of two records of one provider is joined with neither. The Resolver fetches a star's Gaia row by the Gaia DR3 designation its record lists, through `resolve.IdentifierResolver`, and no longer folds a close binary's two Gaia rows into one star. Gaia rows carry Gaia's designation. Each of `catalog.Resolver.PositionMatchThreshold` and `xmatch.WithPositionMatchThreshold` was removed.
