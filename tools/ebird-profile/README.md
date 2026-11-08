# Private eBird seasonal range supplement

This optional tool uses a personal eBird Basic Dataset (EBD) export and its
matching sampling-event file to supplement BirdNET-Go's species range gate.
It does not train an acoustic model, alter confidence scores or thresholds,
or promise an increase in detection accuracy. It can prevent the geographic
filter from rejecting an acoustically valid species regularly reported locally.
Birds already admitted by the geomodel keep their existing behavior, including
rare visitors. Explicit species exclusions still apply.

## Import

Use Python 3.10 or later, with no third-party packages:

```powershell
python tools/ebird-profile/main.py `
  ebd_US-NJ-001_smp_relSep-2026/ebd_US-NJ-001_smp_relSep-2026.txt `
  ebd_US-NJ-001_smp_relSep-2026/ebd_US-NJ-001_smp_relSep-2026_sampling.txt `
  --county US-NJ-001 `
  --citation ebd_US-NJ-001_smp_relSep-2026/recommended_citation.txt `
  --output development-data/atlantic-county.ebird.local.json
```

The supplied region is **Atlantic County**, rather than all of southern New
Jersey. The importer uses the ten calendar years ending in the latest eligible
checklist year (`--years` changes this). It counts complete stationary/traveling
checklists of up to five hours and five kilometers, including checklists on
which a species was absent. Shared group checklists contribute once, using the
lowest numeric sampling-event identifier. Only approved species/subspecies
records count; subspecies are combined with their parent and duplicate species
rows on a checklist count once. Provisional and escapee exotic records are
excluded; naturalized species remain eligible.

See [eBird's complete-checklist guidance](https://cornelllabofornithology.github.io/auk/reference/auk_zerofill.html)
and [shared-checklist deduplication](https://cornelllabofornithology.github.io/auk/reference/auk_unique.html).
Checklist reporting frequency is affected by habitat, observer effort and
visibility; it is neither microphone detectability nor a calibrated probability.

## Enable

Set `birdnet.rangefilter.localdatapath` in the **development** configuration to
the absolute path of the generated profile, or patch
`birdnet.rangeFilter.localDataPath` through the existing v2 settings API. API
changes rebuild the gate immediately. An empty path disables the supplement.
The standard daily range rebuild selects the current local calendar month.
File replacements are observed on the next range-list operation; use the existing
range-filter rebuild control to update the live gate immediately after an import.

A species qualifies when the month has at least 100 independent complete
checklists, at least five reports of that species, and its reporting frequency
meets the configured range-filter occurrence threshold (normally 1%). Only
species supported by a loaded bird classifier can be added. The station must
have a configured location within the sampled-coordinate bounding rectangle;
this is a coverage guard, not a county boundary or habitat model.

The profile supplements startup, daily rebuilds and species previews using the
same helper. Supplemental rows have synthetic-override provenance and cannot
replace native geomodel occurrence/rarity values. Missing, invalid or out-of-area
profiles leave the standard filter operating. The feature is disabled by default.
No detection database, recording or production installation is modified by
importing a profile.

## Privacy and attribution

Exports under `/ebd_*/` and files ending in `.ebird.local.json` are ignored by
Git. Keep them local. Generated profiles contain monthly species counts,
aggregate geographic bounds and the provided citation; they contain no observer
identifiers, checklist identifiers, individual locations or comments. Importing
does not send data to eBird or another service.

The supplied export citation is: eBird Basic Dataset. Version: EBD_relSep-2026.
Cornell Lab of Ornithology, Ithaca, New York. Sep 2026. Read the export's metadata
and `terms_of_use.txt` before use. Derived profiles remain subject to those terms;
retain that file with the private profile and do not redistribute raw exports.

## Verify

```powershell
python -m unittest discover -s tools/ebird-profile -p "test_*.py"
```

Go integration tests cover seasonal selection, evidence cutoffs, location and
exclusion guards, supported labels, cache replacement/removal and concurrent
reads, native-score preservation, and agreement between the live gate, preview
and daily-update method for both geomodel backends.

## Weekly calendar and microphone comparison

`calendar.py` creates a self-contained interactive HTML report from the existing
local-rarity import produced by `scripts/import-local-rarity.py`. Its historical
baseline uses the ten prior calendar years and centered three-week reporting
windows. It identifies sustained arrival/departure transitions and peak periods,
including winter seasons that cross New Year, and distinguishes year-round birds.
Sample-poor windows remain unknown. These are reporting patterns rather than
validated migration dates.

```bash
python3 tools/ebird-profile/calendar.py \
  --history development-data/atlantic-rarity.ebird.local.json \
  --database /path/to/a/read-only-snapshot-of-the-v2-database.db \
  --names internal/imageprovider/data/latest.json \
  --timezone America/New_York \
  --output development-data/bird-calendar.html \
  --json-output development-data/bird-calendar.ebird.local.json
```

Python needs IANA timezone data; WSL/Linux normally supplies it. On Windows,
Python's `tzdata` package supplies those definitions. When reading a Windows
instance through WSL, use a SQLite backup snapshot rather than sharing the live
Windows WAL files across operating systems. The exporter opens the database in
read-only mode, never modifies recordings, and excludes reviewed false positives
and non-species labels. Repeated calls count once per species/day in the days-heard
comparison. All saved station sources are included. Scientific names are matched
exactly; a taxon without a historical match remains unknown.

The calendar supports week selection, common/scientific-name search, seasonal
filters and a heard-this-week filter. Colors show historical checklist frequency;
dots show microphone detections. Weeks without saved station detections have
unknown listening coverage. Its snapshot time and station-record date range are
visible; regenerate it to include later detections. The offline HTML loads without
network access or third-party scripts. Generated HTML and derived data remain
private and Git-ignored.

For the native **Bird calendar** navigation page, set
`BIRDNET_LOCAL_CALENDAR_DATA` in the development backend process to the private
JSON output's absolute path. The authenticated `/api/v2/species/calendar` endpoint
reads the latest snapshot on each request with private/no-store caching. The UI
uses the existing app shell, theme, navigation and translations. Keep microphone
comparison data on the server; never copy it into frontend static assets or
embed it in a distributable binary. The page explains when no import is available.

For automatic updates, add `--watch` to the calendar command and keep it running
alongside the development backend. It generates the initial report immediately
and refreshes every minute, using the current year automatically. Failed reads
preserve the last successful report and retry. Stop the watcher with the backend.
The native tab loads automatically when opened and refreshes every minute while
open, preserving the selected week and filters. Both processes must use the same
private JSON output path; no repeated upload or export is needed.
