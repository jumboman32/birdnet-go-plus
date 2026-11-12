#!/usr/bin/env python3
"""Prepare a local rarity import from authorized eBird EBD and sampling TSVs.

Uses only Python's standard library. Never downloads data, handles credentials,
or connects to BirdNET-Go. See doc/local-rarity.md for data access and caveats.
"""

import argparse
import csv
import datetime
import gzip
import json
import math
from pathlib import Path
import tempfile

EARTH_RADIUS_KM = 6371.0088
MAX_IMPORT_BYTES = 32 << 20


def distance_km(lat1, lon1, lat2, lon2):
    """Great-circle distance, matching the backend's radius check."""
    a, b = math.radians(lat1), math.radians(lat2)
    x = math.sin((b - a) / 2) ** 2 + math.cos(a) * math.cos(b) * math.sin(
        math.radians(lon2 - lon1) / 2
    ) ** 2
    return EARTH_RADIUS_KM * 2 * math.asin(math.sqrt(min(1, x)))


def rows(path, required):
    """Read plain or gzip eBird TSVs, rejecting incompatible column sets."""
    opener = gzip.open if str(path).endswith('.gz') else open
    with opener(path, 'rt', encoding='utf-8-sig', newline='') as stream:
        reader = csv.DictReader(stream, delimiter='\t')
        missing = set(required) - set(reader.fieldnames or [])
        if missing:
            raise ValueError(f'{path}: missing columns: {sorted(missing)}')
        yield from reader


def build_dataset(ebd, sampling, lat, lon, radius, region_code, region_name, source):
    """Keep eligible complete checklists and join their species observations."""
    checklists = {}
    required = ['SAMPLING EVENT IDENTIFIER', 'LOCALITY ID', 'OBSERVATION DATE',
                'LATITUDE', 'LONGITUDE', 'ALL SPECIES REPORTED', 'PROTOCOL CODE',
                'DURATION MINUTES', 'EFFORT DISTANCE KM', 'NUMBER OBSERVERS']
    for row in rows(sampling, required):
        if row['ALL SPECIES REPORTED'] != '1' or row['PROTOCOL CODE'] not in ('P21', 'P22'):
            continue
        if row['PROTOCOL CODE'] == 'P22' and not row['EFFORT DISTANCE KM'].strip():
            continue
        try:
            a, b = float(row['LATITUDE']), float(row['LONGITUDE'])
            duration = float(row['DURATION MINUTES'])
            distance = float(row['EFFORT DISTANCE KM'] or '0')
            observers = int(row['NUMBER OBSERVERS'])
            day = datetime.date.fromisoformat(row['OBSERVATION DATE']).isoformat()
        except (ValueError, TypeError):
            continue
        if not (0 < duration <= 180 and 0 <= distance <= 5 and 0 < observers <= 10
                and -90 <= a <= 90 and -180 <= b <= 180
                and distance_km(lat, lon, a, b) <= radius):
            continue
        event_id = row['SAMPLING EVENT IDENTIFIER']
        if not event_id or not row['LOCALITY ID']:
            continue
        if event_id in checklists:
            raise ValueError(f'Duplicate sampling event: {event_id}')
        checklists[event_id] = dict(
            id=event_id, groupId=row.get('GROUP IDENTIFIER', ''),
            localityId=row['LOCALITY ID'], date=day, latitude=a, longitude=b,
            complete=True, protocol=row['PROTOCOL CODE'], durationMinutes=duration,
            distanceKm=distance, observers=observers, species=set())

    taxa = set()
    joined = set()
    unresolved = set()
    for row in rows(ebd, ['SAMPLING EVENT IDENTIFIER', 'SCIENTIFIC NAME', 'CATEGORY']):
        checklist = checklists.get(row['SAMPLING EVENT IDENTIFIER'])
        if checklist is None:
            continue
        joined.add(checklist['id'])
        if row['CATEGORY'] != 'species':
            # Without an authoritative taxonomy roll-up, subspecies, hybrids,
            # slash taxa and "spuh" records cannot safely become absences for
            # their possible parent species. Exclude the entire event.
            unresolved.add(checklist['id'])
            continue
        name = row['SCIENTIFIC NAME'].strip()
        if name:
            taxa.add(name)
            checklist['species'].add(name)
        else:
            unresolved.add(checklist['id'])
    if not joined:
        raise ValueError('No matching observations: use EBD and sampling files from the same download.')

    # An unmatched event must not become a false all-species non-detection.
    selected, sites, groups = [], set(), set()
    for event_id in sorted(joined):
        if event_id in unresolved:
            continue
        checklist = checklists[event_id]
        site = (checklist['localityId'], checklist['date'])
        group = checklist['groupId']
        if site in sites or (group and group in groups):
            continue
        sites.add(site)
        if group:
            groups.add(group)
        checklist['species'] = sorted(checklist['species'])
        selected.append(checklist)
    if not selected:
        raise ValueError('No fully species-resolved complete checklists remain after filtering.')
    return dict(version=1, source=source,
                retrievedAt=datetime.datetime.now(datetime.timezone.utc).isoformat(),
                regionCode=region_code, regionName=region_name, latitude=lat,
                longitude=lon, radiusKm=radius, taxa=sorted(taxa), checklists=selected)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--ebd', type=Path, required=True)
    parser.add_argument('--sampling', type=Path, required=True)
    parser.add_argument('--latitude', type=float, required=True)
    parser.add_argument('--longitude', type=float, required=True)
    parser.add_argument('--radius-km', type=float, default=25)
    parser.add_argument('--region-code', default='US-NJ-001')
    parser.add_argument('--region-name', default='Atlantic County, New Jersey')
    parser.add_argument('--source', required=True, help='eBird release identifier and citation')
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    if not (-90 <= args.latitude <= 90 and -180 <= args.longitude <= 180
            and 0 < args.radius_km <= 50):
        parser.error('Invalid coordinates or radius (maximum 50 km).')
    if args.output.resolve() in (args.ebd.resolve(), args.sampling.resolve()):
        parser.error('The output must not overwrite either source download.')
    dataset = build_dataset(args.ebd, args.sampling, args.latitude, args.longitude,
                            args.radius_km, args.region_code, args.region_name, args.source)
    encoded = json.dumps(dataset, separators=(',', ':'), allow_nan=False).encode()
    if len(encoded) > MAX_IMPORT_BYTES:
        parser.error('Import exceeds 32 MiB. Prefilter the source files to a smaller local area.')
    # Replace atomically; never leave a partially written import for the server.
    args.output.parent.mkdir(parents=True, exist_ok=True)
    temporary = None
    try:
        with tempfile.NamedTemporaryFile(dir=args.output.parent, delete=False) as stream:
            temporary = Path(stream.name)
            stream.write(encoded)
        temporary.replace(args.output)
    finally:
        if temporary is not None:
            temporary.unlink(missing_ok=True)
    print(f'Wrote {len(dataset["checklists"])} locality-days and {len(dataset["taxa"])} taxa.')


if __name__ == '__main__':
    main()
