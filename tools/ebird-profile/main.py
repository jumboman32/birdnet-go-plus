#!/usr/bin/env python3
"""Build a private monthly occurrence profile from an eBird Basic Dataset export."""

import argparse
import collections
import csv
import datetime as dt
import json
import math
from pathlib import Path
from atomic_output import write_atomic

PROFILE_VERSION = 1
MONTHS = range(1, 13)
DEFAULT_YEARS = 10
MAX_DURATION_MINUTES = 300
MAX_DISTANCE_KM = 5
COMPLETE_PROTOCOLS = {"P21", "P22"}
SPECIES_CATEGORIES = {"species", "issf"}
EXCLUDED_EXOTICS = {"P", "X"}


def rows(path):
    """Stream tab-separated records without interpreting quotes in comments."""
    with Path(path).open(encoding="utf-8-sig", newline="") as source:
        yield from csv.DictReader(source, delimiter="\t", quoting=csv.QUOTE_NONE)


def date_of(row):
    return dt.date.fromisoformat(row["OBSERVATION DATE"])


def event_number(event):
    return int(event.removeprefix("S"))


def eligible(row, county):
    """Use comparable complete stationary/traveling checklists in one county."""
    duration = float(row["DURATION MINUTES"] or "0")
    distance = float(row["EFFORT DISTANCE KM"] or "0")
    return (
        row["COUNTY CODE"] == county
        and row["ALL SPECIES REPORTED"] == "1"
        and row["PROTOCOL CODE"] in COMPLETE_PROTOCOLS
        and math.isfinite(duration)
        and 0 < duration <= MAX_DURATION_MINUTES
        and math.isfinite(distance)
        and 0 <= distance <= MAX_DISTANCE_KM
    )


def build_profile(observations, sampling, county, citation, years=DEFAULT_YEARS):
    """Count independent complete checklists, including those missing a species."""
    if years < 1:
        raise ValueError("years must be positive")
    # Retain only checklist identity, month and coordinates; never observer data.
    candidates = []
    for row in rows(sampling):
        if eligible(row, county):
            date = date_of(row)
            event = row["SAMPLING EVENT IDENTIFIER"]
            group = row["GROUP IDENTIFIER"] or event
            candidates.append((date, event, group, float(row["LATITUDE"]), float(row["LONGITUDE"])))
    if not candidates:
        raise ValueError("no eligible complete checklists in the requested county")
    latest = max(item[0] for item in candidates)
    first_year = latest.year - years + 1
    independent = {}
    for item in candidates:
        date, event, group, _, _ = item
        if date.year < first_year:
            continue
        previous = independent.get(group)
        if previous is None or event_number(event) < event_number(previous[1]):
            independent[group] = item
    selected = {item[1]: item[0].month for item in independent.values()}
    totals = collections.Counter(selected.values())
    counts = {month: collections.Counter() for month in MONTHS}
    seen = set()
    observation_rows = 0
    matched_events = set()
    for row in rows(observations):
        observation_rows += 1
        event = row["SAMPLING EVENT IDENTIFIER"]
        month = selected.get(event)
        if month is not None:
            matched_events.add(event)
        if (
            month is None
            or row["COUNTY CODE"] != county
            or row["APPROVED"] != "1"
            or row["CATEGORY"] not in SPECIES_CATEGORIES
            or row["EXOTIC CODE"] in EXCLUDED_EXOTICS
            or row["OBSERVATION COUNT"] == "0"
        ):
            continue
        # Collapse subspecies rows onto their parent species and deduplicate.
        scientific = " ".join(row["SCIENTIFIC NAME"].split()[:2])
        if len(scientific.split()) != 2:
            continue
        key = (event, scientific)
        if key not in seen:
            counts[month][scientific] += 1
            seen.add(key)
    if not matched_events:
        raise ValueError("no matching observations; use files from the same eBird download")
    latitudes = [item[3] for item in independent.values()]
    longitudes = [item[4] for item in independent.values()]
    if not all(math.isfinite(value) for value in latitudes + longitudes):
        raise ValueError("invalid checklist coordinates")
    return {
        "version": PROFILE_VERSION,
        "region": county,
        "citation": citation.strip(),
        "first_year": first_year,
        "last_year": latest.year,
        "observation_rows": observation_rows,
        "bounds": {
            "south": min(latitudes), "north": max(latitudes),
            "west": min(longitudes), "east": max(longitudes),
        },
        "months": [
            {"month": month, "checklists": totals[month], "species": dict(sorted(counts[month].items()))}
            for month in MONTHS
        ],
    }


def write_profile(profile, output):
    """Replace the output atomically so a running backend never reads half a file."""
    data = (json.dumps(profile, indent=2, allow_nan=False) + "\n").encode("utf-8")
    write_atomic(output, data)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("observations", type=Path)
    parser.add_argument("sampling", type=Path)
    parser.add_argument("--county", required=True)
    parser.add_argument("--citation", required=True, type=Path)
    parser.add_argument("--years", type=int, default=DEFAULT_YEARS)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    if args.output.resolve() in {args.observations.resolve(), args.sampling.resolve(), args.citation.resolve()}:
        parser.error("output must not overwrite a source file")
    profile = build_profile(args.observations, args.sampling, args.county,
                            args.citation.read_text(encoding="utf-8"), args.years)
    write_profile(profile, args.output)
    print(json.dumps({"region": profile["region"], "first_year": profile["first_year"],
                      "last_year": profile["last_year"],
                      "checklists": sum(month["checklists"] for month in profile["months"]),
                      "species": len({name for month in profile["months"] for name in month["species"]})}))


if __name__ == "__main__":
    main()
