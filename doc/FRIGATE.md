# Frigate visual observations

BirdNET-Go+ polls Frigate's [events API](https://docs.frigate.video/integrations/api/events-events-get/)
for `bird` objects. One serial worker writes to the v2 `visual_detections` repository;
`visual_sync_states` holds durable cursors. Visual observations never enter audio
detection tables or audio counts. Both SQLite and the v2 MySQL/MariaDB naming modes
use the existing additive schema initialization. Enabling this feature does not
start a legacy database migration. Stations still using legacy storage must
explicitly migrate through System > Database first.

## Local setup

For visual species identification through MQTT, see the
[YA-WAMF Windows deployment guide](../deployment/yawamf/README.md). It keeps this
poller and dashboard, adds an independent classifier, and avoids duplicate audio
ingestion.

Use the existing backend rather than a separate dashboard. In a supported Linux
development shell (including WSL), from the repository root:

```sh
task setup-dev
task generate-schema
task
./bin/birdnet-go serve
```

Install/setup requires the privileges described by
`task setup-dev`. Frontend development uses `cd frontend && npm run dev` in a
second terminal. See the [development workflow](../.github/CONTRIBUTING.md).

Open Settings > Integrations > Frigate. Enter the URL reachable **from the BirdNET
server**, select cameras (comma separated, initially `feeder`), save, and use
**Test connection**. The URL must include the deployment's actual scheme and port;
there is no hardcoded host. For example, containers sharing a network might use
`http://frigate:5000`. The unauthenticated internal Frigate port must remain on a
trusted network. For authenticated deployments use Frigate's authenticated port
and a trusted TLS certificate; certificate verification is never disabled.

The equivalent settings in the active server `config.yaml` are:

```yaml
realtime:
  frigate:
    enabled: true
    baseurl: "http://frigate:5000"
    username: ""
    password: ""
    token: ""
    cameras: [feeder]
    pollinterval: 20
    snapshotimport: true
    initialhistoryhours: 24
    requesttimeout: 10
    maxattempts: 3
    classificationthreshold: 0
    correlationwindow: 30
    overlapseconds: 3600
    snapshotretentionhours: 168
```

Credentials are server-only YAML settings, omitted entirely from JSON and
preserved across UI saves. They are deliberately not editable or revealable in
the browser. Configure `username` and `password` for
[Frigate native login](https://docs.frigate.video/configuration/authentication/),
or `token` for a pre-issued `frigate_token`. A configured token takes precedence;
the operator must replace it when it expires. Username/password authentication
obtains a fresh session for each synchronization operation. UI changes to
non-secret settings take effect without restarting; changing YAML credentials
requires the application's normal configuration reload/start procedure.

## Import and recovery

Polling defaults to 20 seconds with 100 events per HTTP page. Each scan fixes its
upper time bound, requests ascending event time, and uses Frigate's `offset`
pagination. Each page commits idempotently before the cursor advances. A failure
retains the cursor and replay cannot create duplicate observations. External IDs
are unique within a hashed Frigate URL; cursors also include the selected cameras
and initial-history setting. Camera-selection changes therefore backfill newly
selected cameras safely. A URL change creates a distinct source identity.

Catch-up proceeds in at most 12 elapsed-hour increments per poll, plus overlap.
Requests have configured timeouts and bounded exponential retry delays; scans
have a two-minute deadline and a 1000-page safety limit. Invalid event identity,
timestamps, or response JSON stop that scan without advancing its cursor. HTTP
401/403 are reported without disclosing response bodies or credentials. A
disabled integration makes no event or snapshot requests. Settings changes cancel
an in-flight poll within approximately one second, then join it before restarting.

The default one-hour overlap re-fetches recent events so completed events and
new classifications update existing rows. Frigate has no general event
modification cursor: edits older than the overlap require a backfill. Increase
`initialhistoryhours` to start a new historical scan, or increase `overlapseconds`
to continuously revisit a larger range. This only imports retained Frigate
events, and never rewrites audio detections. Very large overlaps/history windows
can exceed the scan budget; reduce them if status reports repeated failures.

Original sublabels are retained even when unrecognized, ambiguous, or below the
classification threshold. String, tuple-with-score, missing, and malformed
sublabels are handled without inventing a species or score. Exact common-name
resolution uses the existing species index and excludes ambiguous names;
scientific labels are also accepted when present in the index. Unknown scores
remain null. A confidence threshold applies when Frigate provides a score; absent
confidence is not treated as zero. Canonical names depend on the station's loaded
species vocabulary. A classification outside that vocabulary remains unresolved.

## API v2

All routes below use the application's authentication middleware. POST requests
also use the existing global CSRF protection. Settings are available through
`GET /api/v2/settings/frigate` and `PATCH /api/v2/settings/frigate`, or the existing
full-settings API. They never return credentials.

| Method | Route                                        | Behavior                                                                                                                       |
| ------ | -------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| GET    | `/api/v2/integrations/frigate/status`        | Enabled/connected flags, last successful scan, total stored observations, safe last error, process-local poll/failure counters |
| POST   | `/api/v2/integrations/frigate/test`          | Tests saved connection and validates bird event JSON; works before enabling import                                             |
| GET    | `/api/v2/visual-detections`                  | `{data, total, limit, offset}`, ordered newest first                                                                           |
| GET    | `/api/v2/visual-detections/summary`          | Total, unique resolved species, unresolved count, station-local today, top 20 species, UTC hour-of-day counts                  |
| GET    | `/api/v2/visual-detections/:id`              | One visual observation, including original label and independent confidence scores                                             |
| GET    | `/api/v2/visual-detections/:id/snapshot`     | Validated local/proxied JPEG, or a safe unavailable/expired error                                                              |
| GET    | `/api/v2/visual-detections/:id/correlations` | Up to 100 possible audio matches, total count, independent audio confidence and timestamp deltas                               |

List query parameters:

- `limit`: 1–200, default 50; `offset`: nonnegative integer.
- `start`, `end`: RFC3339 instants; start inclusive, end exclusive.
- `camera`: exact camera identifier; `species`: exact scientific name.
- `q`: case-insensitive literal substring of common name, scientific name,
  original label, or camera; maximum 200 bytes. `%` and `_` are literal characters.
- `minConfidence`: classification confidence between 0 and 1; unknown scores do not match.
- `identified`: boolean, based on resolved scientific name.
- `snapshot`: boolean, based on Frigate's reported snapshot availability.

Times in observation responses are UTC. Instants, cursors, and time filters use
Unix microseconds in SQL, matching the backend's numeric timestamp convention.
This avoids SQL connection/server time-zone conversion and DST ambiguity; public
timestamps are derived as UTC instants.

Media availability is a remote report,
not a guarantee that Frigate has not since expired the file. Correlation compares
the visual start with canonical species audio timestamps, initially ±30 seconds.
It can return multiple candidates; unresolved visual species return no candidates.
It stores no relationships and changes no original model scores. Clock differences
must be corrected at the source or accommodated through the comparison window.
Possible matches are not evidence that the same individual bird produced a call.

## Owner species review

`PUT /api/v2/visual-detections/:id/species` accepts a canonical scientific name:
`{"scientificName":"Cardinalis cardinalis","verified":true}`. Authentication
and CSRF protection apply. The species must exist in the active species index.
Success returns 204, unknown IDs return 404, and invalid species return 400.

The review sets `speciesVerified: true` and `classificationSource: "manual"`.
Future Frigate imports preserve the reviewed identity while updating raw labels,
snapshots and times. Manual identity has no invented classifier confidence.
Sending `{"verified":false}` unlocks the record and clears the reviewed identity;
the next eligible poll resolves the current automatic label again.

On the local installation, the following PowerShell example uses the application's
session and CSRF token. For servers requiring sign-in, first authenticate that
same web session through the configured authentication flow.

```powershell
$base = 'http://127.0.0.1:8082'
$app = Invoke-RestMethod "$base/api/v2/app/config" -SessionVariable birdnetSession
$headers = @{ 'X-CSRF-Token' = $app.csrfToken }
Invoke-RestMethod "$base/api/v2/visual-detections?limit=20" -WebSession $birdnetSession
$observationId = Read-Host 'Enter the visual observation ID to review'
$review = @{ scientificName = 'Cardinalis cardinalis'; verified = $true } | ConvertTo-Json
Invoke-RestMethod "$base/api/v2/visual-detections/$observationId/species" -Method Put -WebSession $birdnetSession -Headers $headers -ContentType 'application/json' -Body $review
# To return that same record to automatic classification:
# Invoke-RestMethod "$base/api/v2/visual-detections/$observationId/species" -Method Put -WebSession $birdnetSession -Headers $headers -ContentType 'application/json' -Body '{"verified":false}'
```

Automatic labels expose `classificationSource: "frigate_sublabel"`. Frigate does
not export the producing model ID or a manual-review flag, so this source does
not claim a particular YA-WAMF model. Consult YA-WAMF's own history for model
provenance. Manual edits only in Frigate or YA-WAMF do not create a BirdNET review
lock automatically. The bounded overlap rechecks event start times for one hour
by default; labels written after that window need a deliberate wider replay.

## Snapshots

Snapshot import is off by default. When enabled, authenticated requests download
on demand; completed-event JPEGs are cached under `frigate-snapshots/` next to the
active configuration file. Active events are fetched without caching. Cache names
are hashes, filesystem access is contained by `os.Root`, and writes use temporary
files followed by rename. The cache is separate from exported audio media.

At most two snapshot operations run concurrently. Gallery bursts wait for capacity
instead of failing immediately; waiting and network requests share the configured
request timeout, and canceled requests leave the queue. Compressed images are limited to 8 MiB
and 24 million pixels, checked as JPEG and decoded before serving. The hourly
cleanup removes expired cache artifacts using the latest retention setting; an
expired entry is never served. Missing or expired Frigate snapshots return 404.
Snapshot requests do not download videos or perform additional inference.
If an in-progress snapshot fails, cards and search thumbnails retry when that
event finishes or its snapshot availability changes, without retrying unchanged
failed images on every poll.

## Verification and scope

The mock HTTP tests demonstrate parsing, identified/unidentified observations,
classification updates, duplicate replay, pagination/backfill, durable cursor
recovery, native login/retry, cancellation, UTC conversion, normalization, and
snapshot caching. API tests exercise authentication, filters and correlation;
configuration tests ensure credentials are absent from JSON and cameras are
deep-cloned. No camera, broker, or live Frigate is needed:

```sh
task lint
task test
# Focused iteration with the same test build configuration:
task test TEST_FLAGS='-run Frigate -count=1'
cd frontend
npm run check:all
npm test -- --run src/lib/desktop/features/settings/components/FrigateSettings.test.ts
```

Remaining deployment checks are network reachability, trusted TLS, Frigate camera
and bird-classifier configuration, v2 storage, and manual review of a real imported
event. MySQL/MariaDB must be verified through the supported-release integration
suites; SQLite prefix-mode tests alone do not verify those engines. Poll/status
counters and last-success diagnostics reset on restart; persisted observations
and cursors do not. This phase has no automatic video download, MQTT
subscriber, durable correlation links, or camera-to-microphone mapping.

Implementation files:

- `internal/frigate/{client,auth,service,snapshot}.go` and `frigate_test.go`.
- `internal/datastore/v2/entities/visual_detection.go`, `repository/visual.go`,
  `repository/visual_test.go`,
  and schema lifecycle registration in `manager.go`, `mysql_manager.go`, `startup.go`.
- `internal/api/v2/visual/{visual,correlation,visual_test}.go`, facade `api.go`,
  settings section/credential preservation in `settings.go`, and the settings
  hot-reload coverage registry in `hotreload_coverage_test.go`.
- `internal/conf/frigate.go`, `frigate_test.go`, `config.go`, `config.yaml`,
  `defaults.go`, `validate.go`, `clone.go`; component registration in `internal/errors/errors.go`.
- Frontend `FrigateSettings.svelte` and its tests, `IntegrationSettingsPage.svelte`,
  settings store/defaults, all 16 message locales, generated translation types.
- `config.schema.json`, [configuration reference](wiki/configuration-reference.md),
  its formatter guards in `cmd/gen-schema/markdown.go`,
  the new table entries in [Database Doctor](wiki/database-doctor.md),
  [API catalog](../internal/api/v2/README.md), and this guide.

## Live cameras

When the integration is enabled, open **Live Cameras** in the desktop sidebar (`/ui/cameras`) to watch a selected
Frigate camera with detection boxes. This view is video only; BirdNET-Go+ continues
to analyze audio separately. Pause releases the video connection. Hidden tabs stop
streaming and status polling; visible tabs reconnect automatically.

`GET /api/v2/integrations/frigate/cameras` returns only selected camera names,
frame rates and online status. `GET /api/v2/integrations/frigate/cameras/:camera/live`
proxies Frigate's documented [MJPEG endpoint](https://docs.frigate.video/integrations/api/mjpeg-feed-camera-name-get/)
through BirdNET authentication. Credentials and camera source URLs stay on the
server. The proxy permits two concurrent viewers, bounds each connection to
30 seconds and each frame to 8 MiB/24 megapixels, and does not record or encode
video. The page renews its stream every 25 seconds and polls status every 10 seconds.
Changes to integration settings apply to the next request.

The viewer keeps the video within 70% of the window height and displays the
camera's reported **Detection FPS** beside playback controls. This is Frigate's
processing statistic, not measured playback FPS. **Refresh** reloads camera status
and saved observations and reconnects the video. If the stream disconnects,
**Retry** reconnects immediately; automatic recovery remains enabled. Connection
failures show a disconnected state rather than an indefinite connecting message.

### Source-quality video

**Video quality** offers two choices. The default **Preview · 720p / 3 FPS** uses
the bounded MJPEG proxy with detection boxes. **Source quality** plays the
configured go2rtc video directly through an authenticated, same-origin MSE
WebSocket proxy. BirdNET-Go+ does not resize, transcode or cap its frame rate.
Actual quality depends on the go2rtc source configured in Frigate and the camera's
output. The original video has no rendered detection boxes. Both modes are video
only and leave audio analysis unchanged.

Source playback requires Frigate go2rtc configuration and a browser supporting
the source H.264 or H.265 codec through MediaSource. The camera-named go2rtc stream
is preferred; configured `cameras.<camera>.live.streams` aliases are supported
when that name is absent. Only existing, named streams are accepted. Browser
requests cannot supply arbitrary input URLs, ffmpeg commands or go2rtc messages.
Credentials stay on the backend and upstream error details are not exposed.

`GET /api/v2/integrations/frigate/cameras/:camera/source?codecs=...` upgrades to
WebSocket after authentication and same-origin validation. It negotiates only
allowlisted video codecs and relays fragmented MP4 without saving it. The shared
two-viewer limit applies to both modes; messages and the browser's fragment queue
are bounded to 8 MiB. Old buffered video is trimmed. Connections last at most ten
minutes and the page renews source playback after nine minutes. Pausing, switching
quality/camera, hiding the tab or leaving the page closes the connection.
Disabling the integration, removing the camera or changing credentials closes
active source connections on the next ten-second heartbeat.

An unsupported codec/browser, unavailable source or startup failure returns to
the preview with an explicit message. **Refresh** retries source quality if it
was selected. Source quality is opt-in on each page visit. A missing go2rtc source
does not disable the working preview. Reverse proxies must pass WebSocket
upgrades and preserve the public Host header; Vite's development proxy does this
for `/api` as well. No extra go2rtc host port or camera credentials in browser URLs
are needed.

Source-playback verification uses an isolated 1920×1080, 30 FPS H.264 fixture:
the browser decoded it through the real Go proxy without resizing, and separate
UI checks exercised quality switching, pause/resume, hidden-tab cleanup and tablet
sizing. Deployed checks on ports 8082 and 5173 verified same-origin handshakes,
cross-origin rejection and fallback when go2rtc cannot retrieve the camera.
These checks do not verify the physical camera's current source quality; that
check was deferred by the owner. No test birds or visual observations were added.

Recent imported observations appear below the feed, with snapshot thumbnails and
separate object/species confidence scores. Search Detections searches both audio
and camera observations by default. Its Detection source selector offers All
sources, Audio and Frigate. Species text and date bounds apply to both sources;
advanced audio filters and audio sorting apply to audio results. Camera results
remain newest first and show Frigate as their source. They stay in their own v2
table and are never presented as audio recordings.

Dashboard Recent Detections combines audio and camera cards in timestamp order,
using separate source-qualified IDs and the configured gallery limit. Camera
cards use the event snapshot and a Frigate source label, without audio playback
or audio review controls. The dashboard polls visual observations every 20
seconds while enabled and visible, pauses during card interactions, and refreshes
them with the gallery's manual refresh button. Daily audio summaries are unchanged.
