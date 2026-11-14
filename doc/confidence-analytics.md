# Confidence analytics and station summary

This feature enhances the existing Quality confidence-distribution chart and the
Summary overview. It does not add a second confidence chart to Summary.

## Existing implementation

The Quality chart used the `/api/v2/analytics/confidence/distribution` endpoint
and a shared D3 species ridgeline. It requested twenty normalized bins for the
top five species, ignored the shared species selection, and hid single-species
results. Its v2 implementation loaded individual confidence values before
binning them. Summary already provided species totals, a weighted confidence
average, species distribution, hourly activity, daily trends, new species, and
recent detections, driven by shared date controls.

## Changes

The existing endpoint retains its default response for other clients. The new
`format=analysis` representation uses a read-only v2 repository with SQL-grouped
counts and sums. Exact medians read at most two ordered scores per series within
the same repeatable-read transaction. This avoids downloading raw detections to
calculate statistics. It supports repeated scientific-name `species` filters and
recorded `model` IDs. The API catalog describes both new representations.

Quality uses twenty 5% histogram bins, a counts/percentages toggle, optional
species comparisons, and recorded-model selection. Each model stays separate.
Selecting a bin opens a bounded review list with links to the existing detection
detail and verification workflow. Each review request loads at most 25 records.
Bars support touch, pointer, Enter, and Space activation.

Summary preserves its station totals and charts. Confidence Overview and generated
Summary Insights have been removed from this page.
The Summary view uses 85.5% of the available content width at normal text and control
sizes and includes an activity snapshot:
average confidence percentage, server uptime percentage, detections per selected calendar day, and the
busiest date. An hourly chart shows detection totals for all 24 hours, zero-filling
hours with no detections. The weekday and grouped time-of-day charts have been removed.
The daily average is a calendar metric, not a recording
uptime estimates. The uptime card uses the system API's current process session;
periods partly covered by that session show a minimum confirmed percentage and
an explicit partial-history note. Periods with no overlap show unavailable because
no historical uptime ledger exists. The card refreshes every ten minutes. Today's
denominator ends at the latest observation, rather
than counting future hours as downtime. This measures server availability, not
microphone or audio-stream availability. The species ranking switches between the top 10 and top 20,
remembers the visitor's selection, and displays its share of all detections.
Species bars open detections filtered to the selected dates.
The top-10 view also offers a saved bar/pie preference. Its pie includes an
“Other species” slice so percentages cover all detections rather than only the
leading ten species. Species slices open the same filtered detections and support
keyboard activation. The top-20 view uses bars, and saved top-25 preferences migrate
to top 20.
Activity Snapshot's confidence average weights each species' mean by its detection
count across the entire selected period, including species outside the top-20 chart.
Empty periods show no value rather than a misleading 0% confidence.
The new-species timeline loads every API page for the selected period and scrolls
vertically with readable rows, rather than silently truncating to 20 species.
Its date axis includes the entire final calendar day, including one-day selections.
Detection Trends aligns its range to the daily points' local calendar dates and
insets endpoint markers so the current day's dot remains fully visible, including on hover.

Summary also includes two analytical panels:

- **Patterns in this period** calculates Shannon effective species diversity,
  the top five species' detection share, the number of species contributing 80%
  of detections, and the most active three-hour window (including across midnight).
  A percentage chart shows the five leading species and all remaining species.
  These are patterns in acoustic detections, not estimates of bird abundance.
- **Compared with the previous period** uses adjacent, equal-length calendar
  periods. Today and future days are excluded. It shows changes in detection
  volume and effective diversity, the percentage of previous-period species still
  detected, and the three largest gains and declines in detection share. Share
  changes use percentage points, independent of changes in total detections.
  Species links open detections for the completed comparison dates. Missing
  earlier detections never produce an infinite or fabricated percentage change;
  errors offer a retry and outdated responses cannot replace a newer period.

Comparisons use existing species-summary API responses for both full periods,
independent of the top-10/top-20 display limit. Recording coverage may differ
between periods; the panels do not infer uptime, migration, weather, or causality.

New visible strings are translated in all sixteen supported locales. Cards and
charts use the existing theme variables and D3 components. Request cancellation
and stale-result guards prevent older date ranges replacing newer results.

## Calculation scope

- Every eligible saved primary identification in the selected inclusive local
  calendar dates contributes once. Secondary predictions are not extra detections.
- Reviewed false positives are excluded. Unreviewed and reviewed-correct detections
  remain eligible. Stored scores outside 0–100% are excluded.
- No current minimum-confidence setting is reapplied to historical detections.
  Species overrides and dynamic thresholds can admit scores below a global
  threshold. Historical thresholds are not inferred from today's configuration.
- Model IDs distinguish recorded model identities. Missing model names receive an
  explicit unknown-model label; history is never silently attributed to BirdNET.
- The 20–30% and 30–80% bands exclude their upper bounds. The 80–100% band includes
  100%. Scores below 20% remain in the calculation denominator. Every displayed
  proportion includes its sample size.
- Confidence is a model score, not calibrated identification accuracy. Comparing
  distributions does not establish that one model identifies birds more accurately.

The enhanced v2 database must be active. A legacy database or partially populated
migration target does not supply these new calculations. This feature performs
no migration, schema change, recording change, or production configuration change.

## Deliberate limits

Historical recording uptime is not available from the existing analytics data.
Consequently the dashboard does not invent a detections-per-recording-hour rate
from elapsed calendar hours or current server uptime. Insights describe recorded
data only and do not infer weather, migration, behavior, or recording quality.

The Summary omits the recent-detections feed to keep the page focused on analytics.
Counts can change between a
histogram request and a later review request as detections are reviewed or expire.
