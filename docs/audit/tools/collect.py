"""Collect per-package audit facts for the audit map.

Run from the repository root of an up-to-date main checkout:

    python docs/audit/tools/collect.py [--work DIR] [--cover-from FILE]

Writes DIR/data.json. Every number comes from the repository, the Go
toolchain or GitHub (through the gh CLI), never from memory:

  - packages, from `go list ./...`;
  - lines of code, test files, fuzz targets and build tags, from the sources;
  - findings, from GitHub issues closed by a commit on main whose message says
    "Closes #N", attributed to every package whose Go files that commit changed;
  - statement coverage, from `go test -cover ./...` on the untagged suite, or
    from a saved copy of its output given with --cover-from.
"""
import argparse
import collections
import json
import os
import re
import subprocess
import tempfile

MODULE = "github.com/TuSKan/astrogo"
REPO = "TuSKan/astrogo"

# A test file that names one of these compares against something outside
# astrogo: a service, another library, or a published table.
REFERENCE = re.compile(
    r"(?i)horizons|sofa|skyfield|astropy|erfa|wcslib|sbpy|vallado|usno|astropixels|"
    r"nasa eclipse|leinert|krisciunas|gambons|kocifaj|published|paper|table \d")


def run(*args):
    return subprocess.run(args, capture_output=True, text=True, encoding="utf-8", check=True).stdout


def coverage(cover_from):
    """Statement coverage per package directory, from `go test -cover` output."""
    if cover_from:
        with open(cover_from, encoding="utf-8") as f:
            text = f.read()
    else:
        # check=False: a failing package still reports the coverage of the others.
        text = subprocess.run(["go", "test", "-cover", "./..."], capture_output=True, text=True,
                              encoding="utf-8").stdout
    out = {}
    for line in text.splitlines():
        m = re.search(r"(" + re.escape(MODULE) + r"\S*)\s.*coverage: ([0-9.]+)% of statements", line)
        if m:
            pkg = m.group(1)
            out[pkg[len(MODULE) + 1:] if pkg != MODULE else "."] = float(m.group(2))
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--work", default=os.path.join(tempfile.gettempdir(), "astrogo-auditmap"))
    ap.add_argument("--cover-from", help="saved `go test -cover ./...` output to read instead of running it")
    args = ap.parse_args()
    os.makedirs(args.work, exist_ok=True)

    root = os.getcwd()
    pkgs = [p.strip() for p in run("go", "list", "./...").splitlines() if p.strip()]
    dirs = {(p[len(MODULE) + 1:] if p != MODULE else "."): p for p in pkgs}

    def pkg_of(path):
        """The package directory a repository file belongs to, or None."""
        d = os.path.dirname(path).replace("\\", "/") or "."
        while d not in dirs and d not in (".", ""):
            d = os.path.dirname(d) or "."
        return d if d in dirs else None

    facts = {}
    for r in dirs:
        loc = tests = refs = 0
        fuzz, tags = [], collections.Counter()
        for name in sorted(os.listdir(os.path.join(root, r))):
            if not name.endswith(".go"):
                continue
            with open(os.path.join(root, r, name), encoding="utf-8") as f:
                src = f.read()
            if name.endswith("_test.go"):
                tests += 1
                fuzz += re.findall(r"^func (Fuzz\w+)\(", src, re.M)
                m = re.match(r"\s*//go:build (.+)", src)
                if m:
                    for t in ("validation", "network", "integration"):
                        if t in m.group(1):
                            tags[t] += 1
                if REFERENCE.search(src):
                    refs += 1
            else:
                loc += sum(1 for line in src.splitlines() if line.strip() and not line.strip().startswith("//"))
        facts[r] = {"loc": loc, "test_files": tests, "fuzz": sorted(set(fuzz)), "tags": dict(tags),
                    "ref_test_files": refs}

    log = run("git", "log", "origin/main", "--no-merges", "--format=%x00%H%x01%B%x02", "--name-only")
    closes = collections.defaultdict(set)
    for chunk in log.split("\x00")[1:]:
        head, _, files = chunk.partition("\x02")
        body = head.partition("\x01")[2]
        nums = {int(n) for n in re.findall(r"(?i)\b(?:closes|fixes|resolves)\s+#(\d+)", body)}
        if not nums:
            continue
        touched = {pkg_of(f.strip()) for f in files.split("\n")
                   if f.strip().endswith(".go") and not f.strip().startswith("examples/")}
        touched.discard(None)
        for n in nums:
            closes[n] |= touched

    issues = json.loads(run("gh", "issue", "list", "-R", REPO, "--state", "all", "--limit", "5000",
                            "--json", "number,title,state,labels,closedAt"))
    per_pkg = collections.defaultdict(list)
    unattributed = []
    for i in issues:
        labels = [lb["name"] for lb in i["labels"]]
        if closes.get(i["number"]):
            for p in closes[i["number"]]:
                per_pkg[p].append({"n": i["number"], "title": i["title"], "state": i["state"],
                                   "labels": labels, "closed": i["closedAt"]})
        elif i["state"] == "CLOSED" and "bug" in labels:
            unattributed.append(i["number"])

    data = {
        "head": run("git", "rev-parse", "--short", "origin/main").strip(),
        "date": run("git", "log", "-1", "--format=%cs", "origin/main").strip(),
        "coverage": coverage(args.cover_from),
        "packages": {r: {**facts[r], "issues": sorted(per_pkg.get(r, []), key=lambda x: -x["n"])}
                     for r in sorted(dirs)},
        "open_issues": [{"n": i["number"], "title": i["title"]} for i in issues if i["state"] == "OPEN"],
        "unattributed_closed_bugs": sorted(unattributed),
    }
    out = os.path.join(args.work, "data.json")
    with open(out, "w", encoding="utf-8", newline="") as f:
        json.dump(data, f, indent=1)
    print(f"{out}: {len(dirs)} packages, {len(issues)} issues, "
          f"{sum(1 for n in closes if closes[n])} attributed, {len(unattributed)} closed bugs unattributed")


if __name__ == "__main__":
    main()
