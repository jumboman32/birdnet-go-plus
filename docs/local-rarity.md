# Local Rarity Score

Detection Details shows seasonal **eBird reporting rarity**, separate from BirdNET
identification confidence. BirdCast is an optional external link for migration
context and never changes the score. No settings, recordings, or database rows are
written by this feature.

## One-command preview without a backend

From `frontend/`, run `npm.cmd run dev:rarity` in Windows PowerShell (or
`npm run dev:rarity` elsewhere). Vite opens the rarity preview automatically.
Use the terminal's Local address if your normal development port is occupied.

The preview renders the actual rarity card with clearly labelled synthetic
responses: scores of 75 and 98, insufficient data, historical evidence, and an
unavailable backend with retry. It needs no Go installation, API key, database,
or running production server. Its API proxy is disabled and other API requests
are rejected locally. This tests the interface and states; it does not calculate
rarity for real detections or load historical data. Use the import/backend steps
below for real-data validation. The preview is never included in production builds.

## Data and limitations

The [official eBird data guidance](https://science.ebird.org/en/use-ebird-data)
recommends complete-checklist data, sampling effort, and careful treatment of
sampling bias. Obtain an authorized **all-species** eBird Basic Dataset (EBD) and
its matching Sampling Event Data from [eBird downloads](https://ebird.org/data/download).
Follow the applicable terms and citation requirements. An API key alone does not
provide this historical dataset. Do not use a species-filtered EBD: missing rows
would incorrectly become non-detections. Include the surrounding area in the
download, including adjacent counties where the chosen radius crosses a boundary.

The importer accepts plain or gzip TSV exports. It retains complete stationary
(P21) and traveling (P22) checklists of 1–180 minutes, at most 5 km, and 1–10
observers within the specified radius. It deduplicates shared groups and retains
the first event in event-ID order per locality/calendar date. It excludes sampling
events without matching EBD observations instead of turning them into absences.
Without an authoritative taxonomy roll-up, an event containing any non-species
taxon (including a subspecies, hybrid, slash taxon, or unidentified group) is
excluded entirely. Such rows must not silently become non-detections of a possible
parent species. This conservative restriction can substantially reduce coverage
and introduces additional identification/observer selection bias.
Observer identities and API keys are never included in the generated JSON.

For a detection date, the backend uses a ±14-day seasonal window in **prior
calendar years only**, with at least 100 eligible locality-days across three
years. Seasons wrap across December/January. Each sample's actual calendar is used;
a February 29 target anchors on February 28 in non-leap years. The current year is
excluded even for dates earlier in that year. The
source data's age remains visible: refresh imports when a suitable eBird release
becomes available. The importer does not download or schedule updates itself.

With `k` reporting locality-days out of `n`, the score is
`round(100 × (1 − (k + 0.5) / (n + 1)))`, using Jeffreys smoothing. A score near 95
means roughly 5% reporting frequency in this restricted historical sample, not a
95% probability of an unusual identification. Zero reports do not establish
absence. The UI shows an approximate 95% Wilson sampling interval transformed to
the score scale. Rounding can yield 0 or 100; these are display endpoints, not
certainty. The interval does not capture habitat bias, spatial dependence,
taxonomic changes, climate trends, or differing observer skills. Effort is handled
by restriction, **not** a fitted detection/occupancy model. Thresholds and the score
mapping are provisional and need evaluation with real local data; this is not a
validated biological rarity index.

Scientific names must match an imported species-level taxon exactly. Unmapped
taxa, sparse seasons, missing imports, and station mismatches show **Insufficient
data**. Unknown taxa are never assigned a score of 100.

## Prepare a local import

Use a development copy of the application. Keep licensed downloads and generated
data outside the repository. Substitute the exact configured station coordinates;
the values below are only a Linwood-area example.

```powershell
python scripts/import-local-rarity.py `
  --ebd C:\BirdData\ebd.txt.gz `
  --sampling C:\BirdData\sampling.txt.gz `
  --latitude 39.34 --longitude -74.57 --radius-km 25 `
  --region-code US-NJ-001 --region-name "Atlantic County, New Jersey" `
  --source "eBird Basic Dataset, RELEASE IDENTIFIER; REQUIRED CITATION" `
  --output C:\BirdData\local-rarity.json

$env:BIRDNET_LOCAL_RARITY_DATA = 'C:\BirdData\local-rarity.json'
$env:BIRDNET_RARITY_REGION = 'US-NJ-001'
```

Set these variables in the **development backend's process**, then start that
backend using its own configuration, data directories, and port. Do not change the
production process or point a development backend at the production database.
The Vite frontend must reach this development backend for the new endpoint to
work; a production backend without the endpoint shows a retryable unavailable
state. Windows environment variables shown here affect only the current shell and
processes started from it.

The generated file is atomically replaced and limited to 32 MiB. Parsed imports
and failures are cached for one minute, so replacement takes effect without a
restart. A failed refresh does not silently serve an older import. Imports must
be centered within 1 km of the current configured station; location changes
otherwise disable their scores. The survey circle remains centered on the imported
coordinates, so coordinate rounding never silently removes a boundary crescent.
The dataset's radius and source are shown in the
card. A regional name is an attribution label, not a county-only spatial filter.

## Automatic supporting evidence

Enable the existing eBird integration and configure its API key on the development
backend through the existing settings flow. Keys stay server-side. The existing
client handles caching, rate limiting, and deduplicating simultaneous requests.
This feature never fetches eBird directly from the browser.

For detections dated today in the station timezone, the backend checks the recent
nearby eBird response over the past 14 days. It uses eBird's default nearby radius
and caps results at 200. A returned report is supporting presence evidence; a
missing report establishes neither absence nor rarity. The response has no
complete-checklist denominator and does not change the numeric score. For older
detections, today's recent observations are not substituted. API failures leave
historical scores available and mark recent evidence unavailable.

The BirdNET model panel remains separate and explicitly describes **current**
seasonal model context; it is not evidence for a historical detection date and is
not part of this score. Its existing API includes coverage/threshold semantics
that make it unsuitable for treating as an independent calibrated frequency.

## BirdCast

`BIRDNET_RARITY_REGION=US-NJ-001` optionally enables the
[Atlantic County dashboard](https://dashboard.birdcast.org/region/US-NJ-001), even
without an eBird import. A matching import supplies its own region instead.
Change/remove this explicit region setting if the station moves. Links select the
**preceding night's evening date**, stated in the label, rather than claiming to
match the detection's exact nocturnal period. BirdCast may not provide data for
the requested date or season. No radar counts are scraped, cached, attributed to
a species, or included in the score. No BirdCast key is required.

## API and checks

`GET /api/v2/species/local-rarity?scientific_name=Cardinalis%20cardinalis&date=2026-05-01`
returns `status`, nullable `score`/`scoreLow`/`scoreHigh`, sample/report/year counts,
source metadata, recent-evidence status, and an optional BirdCast link. It follows
the existing API v2 private-mode access policy. Invalid inputs return 400; missing
data returns 200 with an explanation, not a fabricated number.

```sh
python3 -m unittest discover -s scripts -p 'test_import_local_rarity.py'
go test -race ./internal/rarity
# Run the species package using the repository's normal CGO setup and test tags:
go test -race -tags noembed,skipfrontend ./internal/api/v2/species
cd frontend
npm run test:ci -- src/lib/desktop/views/LocalRarityCard.test.ts src/lib/desktop/views/DetectionDetail.test.ts
```
