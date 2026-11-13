"""Turn collect.py's facts into the audit map's three tables.

    python docs/audit/tools/build.py [--work DIR]

Reads DIR/data.json and writes DIR/packages.json (one row per package: depth
inputs, coverage, gaps), DIR/findings.json (one row per finding and package)
and DIR/sweeps.json. The judgments that are not measured live in judgments.py.
"""
import argparse
import collections
import json
import os
import re
import sys
import tempfile

sys.dont_write_bytecode = True  # no __pycache__ beside the docs
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import judgments  # noqa: E402

# The kind of defect an issue's title names. Ordered: the first match wins, so
# the classification is approximate and "other" holds titles nothing matched.
KINDS = [
    ("nonfinite", "Non-finite or out-of-range inputs",
     r"(?i)\bnan\b|non-?finite|infinit|\binf\b|out of range|negative|zero|overflow|underflow|±inf|divide"),
    ("cache", "Cache, download and data integrity",
     r"(?i)cache|cached|heal|corrupt|download|consent|offline|etag|stale|archive|bulletin|refus|error page|truncat"),
    ("parse", "Parsing third-party formats",
     r"(?i)pars|column|row|header|csv|votable|fits|tiff|tle|format|field|decode|malformed"),
    ("accuracy", "Numerical accuracy against references",
     r"(?i)arcsec|accuracy|horizons|sofa|skyfield|precision|off by|error of|wrong (position|value|sign)|bias|"
     r"drift|km\b|\bmas\b|leap|delta-?t|ut1|tdb|epoch|nutation|refraction|ephemeris"),
    ("concurrency", "Concurrency and resources",
     r"(?i)race|concurren|lock|goroutine|leak|close|deadlock|parallel|mutex"),
    ("perf", "Performance and allocation", r"(?i)alloc|slow|perf|faster|benchmark|quadratic|memory|cpu|recomput"),
    ("api", "API design and errors", r"(?i)error|sentinel|panic|api|signature|return|nil|interface|option"),
    ("docs", "Docs and tests against code",
     r"(?i)doc|readme|claude\.md|comment|example|test|claim|changelog|validation\.md"),
]
KIND_LABEL = {k: label for k, label, _ in KINDS} | {"other": "Other"}


def kind_of(title):
    for k, _, pat in KINDS:
        if re.search(pat, title):
            return k
    return "other"


def layer(p):
    if p.startswith("internal/"):
        return "Internal support"
    if p.startswith("skybrightness/dataset") or p == "atmosphere/dataset/cams":
        return "Dataset providers (I/O)"
    if p in ("plan", "fits/fitsplan", "skybrightness/skyplan") or p.startswith("catalog"):
        return "Orchestration"
    if p.split("/")[0] in ("time", "angle", "vector", "unit", "constants", "remote", "optics", "logging"):
        return "Primitives"
    return "Scientific engines"


def depth(findings, kinds):
    if findings == 0:
        return "Untouched"
    if findings >= 10 and kinds >= 4:
        return "Deep"
    if findings >= 4 or kinds >= 3:
        return "Moderate"
    return "Light"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--work", default=os.path.join(tempfile.gettempdir(), "astrogo-auditmap"))
    args = ap.parse_args()
    with open(os.path.join(args.work, "data.json"), encoding="utf-8") as f:
        d = json.load(f)

    # Open issues, attributed to the package their title names first: by its
    # path, or by its last element when no other package ends in the same one,
    # since titles name a package as its own clause does ("metrology:", not
    # "internal/metrology:").
    by_name = collections.defaultdict(list)
    for p in d["packages"]:
        by_name[p.rsplit("/", 1)[-1]].append(p)
    open_by_pkg = collections.defaultdict(list)
    for i in d["open_issues"]:
        m = re.match(r"^(?:\w+\()?([a-z0-9/]+)\)?[:(]", i["title"])
        if not m:
            continue
        name = m.group(1)
        p = name if name in d["packages"] else (by_name[name][0] if len(by_name[name]) == 1 else None)
        if p:
            open_by_pkg[p].append(f"#{i['n']} {i['title']}")

    packages, findings = [], []
    for p, v in d["packages"].items():
        tc = collections.Counter()
        last = ""
        for i in v["issues"]:
            k = kind_of(i["title"])
            tc[k] += 1
            last = max(last, (i["closed"] or "")[:10])
            findings.append({"package": p, "issue": i["n"], "title": i["title"], "theme": k,
                             "theme_label": KIND_LABEL[k], "state": i["state"], "closed": (i["closed"] or "")[:10]})
        n = len(v["issues"])
        kinds = sum(1 for k, c in tc.items() if c and k != "other")
        cov = d["coverage"].get(p)
        gaps = []
        if p in judgments.EXTERNAL and not v["fuzz"]:
            gaps.append("Parses external bytes but has no fuzz target of its own.")
        if cov is not None and cov < 75:
            gaps.append(f"Statement coverage {cov:.1f}%, below 75%.")
        if p in judgments.NUMERIC and tc["nonfinite"] == 0 and p not in judgments.SWEPT_CLEAN:
            gaps.append("No finding about non-finite inputs yet.")
        if v["loc"] >= 500 and n / (v["loc"] / 1000) < 3 and p not in judgments.AUDITED:
            gaps.append(f"Light audit for its size: {n} findings in {v['loc']} lines.")
        if n == 0 and v["loc"] > 0 and p not in judgments.AUDITED:
            gaps.append("No audit finding has ever touched it.")
        if p in judgments.NOTED:
            gaps.append(judgments.NOTED[p])
        gaps += ["Open: " + o for o in open_by_pkg.get(p, [])]
        packages.append({
            "package": p, "layer": layer(p), "loc": v["loc"], "test_files": v["test_files"],
            "fuzz_targets": len(v["fuzz"]), "fuzz_names": ", ".join(v["fuzz"]),
            "validation_tests": v["tags"].get("validation", 0), "network_tests": v["tags"].get("network", 0),
            "integration_tests": v["tags"].get("integration", 0), "reference_test_files": v["ref_test_files"],
            "coverage": cov, "findings": n, "themes_explored": kinds, "depth": depth(n, kinds),
            **{k: tc[k] for k in KIND_LABEL}, "last_finding": last or None,
            "external_bytes": p in judgments.EXTERNAL, "gaps": " | ".join(gaps), "gap_count": len(gaps),
        })

    # A judgment that no longer applies is refused rather than shown: a bug
    # recorded as fixed outside the packages that a commit now attributes, or
    # that is not a closed bug at all, means the record is out of date.
    stale = sorted(set(judgments.OUTSIDE_PACKAGES) - set(d["unattributed_closed_bugs"]))
    if stale:
        raise SystemExit("judgments.OUTSIDE_PACKAGES names " + ", ".join(f"#{n}" for n in stale) +
                         ", which collect.py no longer reports as closed bugs it cannot attribute")

    meta = {"head": d["head"], "date": d["date"],
            "unattributed_closed_bugs": [n for n in d["unattributed_closed_bugs"]
                                         if n not in judgments.OUTSIDE_PACKAGES],
            "outside_packages": [{"issue": n, "where": judgments.OUTSIDE_PACKAGES[n]}
                                 for n in d["unattributed_closed_bugs"] if n in judgments.OUTSIDE_PACKAGES],
            "distinct_findings": len({f["issue"] for f in findings})}
    for name, rows in (("packages", packages), ("findings", findings), ("sweeps", judgments.SWEEPS),
                       ("meta", meta)):
        with open(os.path.join(args.work, f"{name}.json"), "w", encoding="utf-8", newline="") as f:
            json.dump(rows, f, ensure_ascii=False)
    print(f"main {d['head']}: {len(packages)} packages, {meta['distinct_findings']} findings "
          f"({len(findings)} package rows), {sum(1 for p in packages if p['gap_count'])} packages with gaps")


if __name__ == "__main__":
    main()
