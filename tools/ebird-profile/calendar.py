#!/usr/bin/env python3
"""Export a private, offline weekly bird calendar and read-only microphone comparison."""

import argparse
import collections
from contextlib import closing
import datetime as dt
import json
from pathlib import Path
import sqlite3
import sys
import time
from zoneinfo import ZoneInfo
from atomic_output import write_atomic

WEEKS = 52
DAYS_PER_WEEK = 7
REFERENCE_YEAR = 2001  # A non-leap calendar keeps month/day aligned across years.
HISTORY_YEARS = 10
MINIMUM_SAMPLES = 100
MINIMUM_YEARS = 3
EXPECTED_FLOOR = 0.02
SEASON_FRACTION = 0.2
PEAK_FRACTION = 0.8
REFRESH_SECONDS = 60


def week_of(date):
    """Use Jan 1 anchored seasonal weeks; the final period includes Dec 31."""
    day = min(date.day, 28) if date.month == 2 else date.day
    anchor = dt.date(REFERENCE_YEAR, date.month, day)
    return min((anchor.timetuple().tm_yday - 1) // DAYS_PER_WEEK, WEEKS - 1)


def seasonal_events(rates):
    """Describe sustained seasonal presence, including seasons crossing New Year."""
    peak = max((rate for rate in rates if rate is not None), default=0)
    if peak < EXPECTED_FLOOR:
        return {"arrivals": [], "departures": [], "peaks": [], "resident": False}
    threshold = max(EXPECTED_FLOOR, SEASON_FRACTION * peak)
    present = [None if rate is None else rate >= threshold for rate in rates]
    arrivals, departures, peaks = [], [], []
    for week, expected in enumerate(present):
        previous = present[(week - 1) % WEEKS]
        following = present[(week + 1) % WEEKS]
        if expected is True and previous is False and following is True:
            arrivals.append(week)
        if expected is False and previous is True and following is False:
            departures.append(week)
        if rates[week] is not None and rates[week] >= PEAK_FRACTION * peak:
            peaks.append(week)
    return {"arrivals": arrivals, "departures": departures, "peaks": peaks,
            "resident": all(value is True for value in present)}


def historical_calendar(dataset, year):
    """Estimate checklist reporting frequency with a centered three-week window."""
    totals = [0] * WEEKS
    years = [set() for _ in range(WEEKS)]
    reports = collections.defaultdict(lambda: [0] * WEEKS)
    seen_sites, seen_groups = set(), set()
    for checklist in dataset["checklists"]:
        date = dt.date.fromisoformat(checklist["date"])
        if not year - HISTORY_YEARS <= date.year < year or not checklist["complete"]:
            continue
        site = (checklist["localityId"], date)
        group = checklist.get("groupId")
        if site in seen_sites or group and group in seen_groups:
            continue
        seen_sites.add(site)
        if group:
            seen_groups.add(group)
        week = week_of(date)
        totals[week] += 1
        years[week].add(date.year)
        for name in set(checklist["species"]):
            reports[name][week] += 1
    windows = [[(week + shift) % WEEKS for shift in (-1, 0, 1)] for week in range(WEEKS)]
    samples = [sum(totals[index] for index in window) for window in windows]
    year_counts = [len(set().union(*(years[index] for index in window))) for window in windows]
    rates = {}
    for name, counts in reports.items():
        rates[name] = [
            round(sum(counts[index] for index in window) / samples[week], 4)
            if samples[week] >= MINIMUM_SAMPLES and year_counts[week] >= MINIMUM_YEARS else None
            for week, window in enumerate(windows)
        ]
    return rates, samples, year_counts


def microphone_comparison(database, year, timezone):
    """Read v2 detections without opening a writable connection or touching media."""
    days = collections.defaultdict(lambda: [set() for _ in range(WEEKS)])
    counts = collections.defaultdict(lambda: [0] * WEEKS)
    station_days = [set() for _ in range(WEEKS)]
    query = """
        SELECT d.detected_at, l.scientific_name
        FROM detections d JOIN labels l ON l.id=d.label_id
        JOIN label_types t ON t.id=l.label_type_id
        WHERE t.name='species'
          AND d.detected_at >= ? AND d.detected_at < ?
          AND NOT EXISTS (SELECT 1 FROM detection_reviews r
                          WHERE r.detection_id=d.id AND r.verified='false_positive')
    """
    start = int(dt.datetime(year, 1, 1, tzinfo=timezone).timestamp())
    end = int(dt.datetime(year + 1, 1, 1, tzinfo=timezone).timestamp())
    with closing(sqlite3.connect(Path(database).resolve().as_uri() + "?mode=ro", uri=True)) as source:
        for timestamp, name in source.execute(query, (start, end)):
            date = dt.datetime.fromtimestamp(timestamp, timezone).date()
            if date.year != year:
                continue
            week = week_of(date)
            days[name][week].add(date.isoformat())
            counts[name][week] += 1
            station_days[week].add(date.isoformat())
    all_days = sorted(set().union(*station_days))
    return {name: [len(week) for week in weeks] for name, weeks in days.items()}, dict(counts), [len(week) for week in station_days], all_days


def build_report(dataset, database, names, year, timezone, now):
    rates, samples, years = historical_calendar(dataset, year)
    detected, counts, station_days, dates = microphone_comparison(database, year, timezone)
    species = []
    for name in sorted(set(rates) | set(detected)):
        frequency = rates.get(name, [None] * WEEKS)
        species.append({"scientific": name, "name": names.get(name, name), "rates": frequency,
                        "heard": detected.get(name, [0] * WEEKS), "counts": counts.get(name, [0] * WEEKS),
                        **seasonal_events(frequency)})
    return {"version": 1, "year": year, "currentWeek": week_of(now.date()), "generated": now.isoformat(),
            "timezone": str(timezone), "region": dataset["regionName"], "radius": dataset["radiusKm"],
            "citation": dataset["source"], "firstHistoricalYear": year - HISTORY_YEARS,
            "lastHistoricalYear": year - 1, "species": species, "samples": samples, "years": years,
            "stationDays": station_days, "firstDetectionDate": dates[0] if dates else None,
            "lastDetectionDate": dates[-1] if dates else None,
            "totalDetections": sum(sum(values) for values in counts.values())}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--history", required=True, type=Path)
    parser.add_argument("--database", required=True, type=Path)
    parser.add_argument("--names", type=Path)
    parser.add_argument("--year", type=int)
    parser.add_argument("--timezone", default="America/New_York")
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--json-output", type=Path, help="Private report consumed by the calendar API")
    parser.add_argument("--watch", action="store_true", help="Refresh automatically every minute until stopped")
    args = parser.parse_args()
    inputs = {args.history.resolve(), args.database.resolve()}
    if args.names:
        inputs.add(args.names.resolve())
    if args.output.resolve() in inputs:
        parser.error("output must not overwrite an input")
    if args.json_output and args.json_output.resolve() in inputs | {args.output.resolve()}:
        parser.error("JSON output must not overwrite an input or the HTML report")
    if args.year is not None and not 1 <= args.year < dt.MAXYEAR:
        parser.error("year is outside the supported calendar range")
    timezone = ZoneInfo(args.timezone)
    while True:
        try:
            export_report(args, timezone)
        except (OSError, ValueError, sqlite3.Error):
            if not args.watch:
                raise
            # Preserve the last good atomic report and retry; do not expose private paths.
            print("Calendar refresh failed; retrying in one minute", file=sys.stderr, flush=True)
        if not args.watch:
            break
        time.sleep(REFRESH_SECONDS)


def export_report(args, timezone):
    """Atomically replace derived reports after a successful read-only comparison."""
    now = dt.datetime.now(timezone)
    dataset = json.loads(args.history.read_text(encoding="utf-8"))
    if dataset.get("version") != 1 or not dataset.get("checklists"):
        raise ValueError("a validated local-rarity import is required")
    names = {}
    if args.names:
        names = {entry["sciName"]: entry["name"] for entry in json.loads(args.names.read_text(encoding="utf-8"))}
    report = build_report(dataset, args.database, names, args.year or now.year, timezone, now)
    encoded = json.dumps(report, separators=(",", ":"), allow_nan=False).replace("<", "\\u003c")
    template = Path(__file__).with_name("calendar.html").read_text(encoding="utf-8")
    write_atomic(args.output, template.replace('"__CALENDAR_DATA__"', encoded).encode("utf-8"))
    if args.json_output:
        write_atomic(args.json_output, encoded.encode("utf-8"))
    print(json.dumps({"calendar_species": len(report["species"]), "detections_compared": report["totalDetections"],
                      "station_first_day": report["firstDetectionDate"], "station_last_day": report["lastDetectionDate"]}), flush=True)


if __name__ == "__main__":
    main()
