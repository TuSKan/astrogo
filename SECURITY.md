# Security policy

## Supported versions

astrogo is before 1.0. Only the latest release receives security fixes, as an out-of-band patch release of its minor version rather than with the weekly release train. An older minor version is not patched; upgrade to the latest.

## Reporting a vulnerability

Please do not open a public issue. Report it privately through GitHub's private vulnerability reporting: the repository's **Security** tab, then **Report a vulnerability**. The report and its discussion stay private until a fix is released, and the advisory is published with it.

Include what you can of:

- the astrogo version or commit, and the package;
- the input that triggers it: a file, a server's answer, or the calls;
- what happens (a crash, a hang, memory or disk use without bound, a file written where it should not be, a download the caller's policy refused) and what an attacker gains.

## What is in scope

astrogo reads bytes that someone else controls, and treats these as vulnerabilities:

- **A crash, a hang or unbounded memory use on hostile input.** That covers the answers of the services it queries (SIMBAD, VizieR, Gaia, MAST, JPL, CelesTrak, FINK and the rest) and the files it downloads or a caller hands it: SPK kernels, FITS files, TLEs, the IERS bulletin, catalog CSVs, GeoTIFF and NetCDF files. Every parser of such bytes is fuzzed; `CLAUDE.md` lists them.
- **Defeating the I/O policy:** a download the caller's policy refused (`remote.SetPolicy`), a request while offline (`remote.SetOffline`), or a write outside the cache.
- **A dependency vulnerability that astrogo's code actually reaches.**

Out of scope, and welcome as ordinary issues: a wrong astronomical result, an outage of a third-party service, and a vulnerability in a dependency that astrogo does not call.
