---
type: Changed
pr: 1236
---
**`remote.Endpoint.Mirrors`** is never registered for an endpoint that carries a token, since a mirror is asked through the endpoint's transport, token included. VizieR's and ESA's Gaia archive's entries say why they have none: VizieR's mirrors serve no TAP, and Gaia's ARI and AIP copies refuse the format `catalog/gaia` asks ESA for.
