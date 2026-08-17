# Draft email — GAMBONS data request

**To:** Eduard Masana (corresponding author, ICCUB) — `eduard.masana@ub.edu`
**Cc:** Salvador Bará (USC), Josep Manel Carrasco (ICCUB), Salvador J. Ribas (ICCUB / Parc Astronòmic Montsec)

> Addresses above are inferred from the papers' affiliations — please confirm them before
> sending. Masana is the marked corresponding author on the IJSL paper.

**Subject:** Request for the GAMBONS extra-atmospheric radiance map (open-source Go library)

---

Dear Dr Masana,

I am writing to ask whether the tabulated extra-atmospheric radiance map underlying GAMBONS
could be made available for use in an open-source project.

I maintain **astrogo** (https://github.com/TuSKan/astrogo), an MIT-licensed astronomy and
observation-planning library in Go. It includes a spectral, all-sky night-sky-brightness
engine that predicts ground-level spectral radiance from an explicit atmospheric state. It
currently implements scattered moonlight (Kieffer & Stone 2005 ROLO reflectance propagated
through molecular and aerosol scattering, with Winkler 2022's multiple-scattering
correction) and artificial skyglow (Kocifaj, Bará & Falchi 2022, with sources from VIIRS
annual composites). The natural moonless sky is the component still missing.

Specifically, I would like to ask about the data behind **Table 3 of Masana et al. (2021),
MNRAS 501, 5443**: the radiance outside the Earth's atmosphere, per HEALPix pixel, combining
integrated starlight, diffuse galactic light and extragalactic background light. The paper
notes the full table is available online, and Masana et al. (2022, IJSL) mentions that a
standalone version of GAMBONS is available on request. I was unable to locate a direct
download from https://gambons.fqa.ub.edu, which appears to serve the interactive calculator
rather than the underlying table.

I should be clear that this is not the web tool's own download, which I have used. That
returns azimuth, altitude and mag arcsec⁻² per pixel — the sky *after* GAMBONS' atmospheric
propagation. It is very useful to me as an independent check, and I intend to validate
astrogo against it. But it cannot serve as an input: astrogo applies its own attenuation and
scattering from an explicit aerosol and molecular state, so consuming a pre-propagated
result would apply an atmosphere twice.

What I am asking for is the **extra-atmospheric** map that sits upstream of that step —
order 8 (nside 256), 786,432 pixels, integrated starlight plus diffuse galactic and
extragalactic light, in whichever bands are convenient, indexed by HEALPix pixel or galactic
coordinates. A plain text or FITS table would be ideal. This is the table sampled in Table 3
of the 2021 paper.

On attribution and licensing: astrogo cites primary sources in code, in its design document
and in each component's machine-readable provenance record, which travels with every
computed result. GAMBONS and Masana et al. (2021) would be cited that way. I would follow
whatever terms you prefer, including redistribution restrictions — if the data may not be
redistributed, I would have the library fetch it from a URL you designate rather than
bundling it, which is how astrogo already handles IERS, JPL, VIIRS and CALSPEC data.

I am happy to share what the implementation looks like, or to contribute anything useful
back — the library already has a tested HEALPix implementation and the atmospheric
scattering layer this would plug into.

Thank you for making GAMBONS available, and for considering this.

Best regards,

[name]
[affiliation, if any]
https://github.com/TuSKan/astrogo

---

## Notes before sending

- **Preempt the obvious reply.** The web tool *does* offer a download — the user guide says
  so — but it returns the propagated sky (az, alt, mag arcsec⁻²), not the extra-atmospheric
  map. Without saying this up front, the natural answer is "just use the download button",
  and a second email is needed to explain why that does not work.
- **Ask only for what is needed.** The extra-atmospheric map, not the full standalone model.
  It is a smaller favour, and it is genuinely what astrogo needs.
- **Offer the validation.** Saying you intend to check astrogo against their published tool
  is true, is a reason they should want this to happen, and costs nothing.
- **The resolution wording matters.** Masana et al. write "resolution equal to 8", meaning
  HEALPix *order* 8, i.e. nside = 2⁸ = 256. That gives their quoted 786,432 pixels and
  1.5979×10⁻⁵ sr. Saying "nside 8" would ask for 768 pixels — a thousand times coarser.
- **Raise licensing first, unprompted.** Researchers are usually more willing to share when
  redistribution terms are settled up front, and astrogo's `remote` layer already supports
  fetch-from-source rather than bundling.
- If there is no reply in a few weeks, the fallbacks are the MNRAS supplementary material
  for the 2021 paper, or building the map from a Gaia DR3 bulk aggregation
  (`https://cdn.gea.esac.esa.int/Gaia/gdr3/gaia_source/`, 1,097 files, ~600 GB) as an
  offline job.
