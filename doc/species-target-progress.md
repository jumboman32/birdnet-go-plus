# County and state species progress

The Species page displays two lifetime totals:

- County: detected target species / eBird county species total (percentage)
- State: detected target species / eBird state species total (percentage)

The denominators come from eBird's all-history regional species lists, using its
[regional species-list API](https://documenter.getpostman.com/view/664302/S1ENwy59).
They do not use the recent-observations feed. Current taxonomy entries are limited
to the species category, excluding hybrids and unidentified species groups.
Repeated codes and scientific names count once. The numerator intersects each
target list with the station's distinct lifetime detected scientific names;
reviewed false positives are excluded through the v2 detection repository.

These totals deliberately do not change with the Species page's date, text,
sorting, or confidence controls. Each card identifies its region and lifetime
scope. An empty target list has no percentage, rather than an invented zero.

The county defaults to the existing `BIRDNET_RARITY_REGION` configuration. A visitor
can enter an eBird county code, for example `US-NJ-001`, on the page; the matching
state code is derived from it. This browser preference does not modify server
configuration. County syntax is validated before requests are sent.

The existing eBird integration and API key must be enabled, and the enhanced v2
database must be active. Missing prerequisites are explained on the page. Lists
are cached with the existing eBird cache lifetime and a bounded regional cache;
failed requests never become successful zero totals. Changing counties cancels
the previous request. Credentials remain on the server.

The read-only `/api/v2/species/target-progress` endpoint returns a status and
county/state objects with `region`, `detected`, `total`, and nullable `percentage`.
It creates no schema, settings, migration, or recording changes.
