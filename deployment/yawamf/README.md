# YA-WAMF species classification on Windows

BirdNET-Go+ remains the dashboard and audio analyzer. Frigate detects `bird`,
publishes `frigate/events`, and YA-WAMF classifies its snapshot and writes a
species sublabel back to Frigate. BirdNET-Go+ polls that same event and updates
its existing visual observation. No second audio importer is involved.

This overlay uses the official [v2.23.2 release](https://github.com/Jellman86/YetAnother-WhosAtMyFeeder/releases/tag/v2.23.2)
and its [monolithic Compose file](https://github.com/Jellman86/YetAnother-WhosAtMyFeeder/blob/v2.23.2/docker-compose.monolith.yml),
with the supported `v2.23.2-cpu` image. It adds persistent Docker-managed Linux
volumes and a private Eclipse Mosquitto 2.0.22 broker to the existing
`birdcam_default` network. It does not manage or replace Frigate or MediaMTX.

## Prepare and start

Run these commands in PowerShell with Docker Desktop running Linux containers.
The first block is for a new installation; do not overwrite an existing `.env`.

```powershell
Set-Location C:\Users\greg2\Documents\birdnet-go-plus
$env:Path = "$env:LOCALAPPDATA\Programs\DockerDesktop\resources\bin;$env:Path"
New-Item -ItemType Directory -Force deployment\yawamf\.runtime | Out-Null
Invoke-WebRequest -Uri 'https://raw.githubusercontent.com/Jellman86/YetAnother-WhosAtMyFeeder/v2.23.2/docker-compose.monolith.yml' -OutFile deployment\yawamf\.runtime\docker-compose.monolith.yml
if (!(Test-Path deployment\yawamf\.runtime\.env)) {
    Copy-Item deployment\yawamf\.env.example deployment\yawamf\.runtime\.env
}
Copy-Item deployment\yawamf\mosquitto.conf deployment\yawamf\.runtime\mosquitto.conf
notepad deployment\yawamf\.runtime\.env
```

Verify `DOCKER_NETWORK` and `FRIGATE_URL` match your existing Docker network and
Frigate container name. Leave the app bound to `127.0.0.1`. The internal Frigate
HTTP API is port 5000; the browser's HTTPS port 8971 is a different endpoint.
Keep any credentials in the ignored `.runtime/.env`, never in the overlay or
example. Restrict `.runtime` to your Windows account through **Properties >
Security** if it contains credentials. Do not print expanded Compose configuration
in shared logs because it can contain environment secrets.

```powershell
$composeArgs = @(
    'compose', '-p', 'birdnet-yawamf',
    '--env-file', 'deployment/yawamf/.runtime/.env',
    '-f', 'deployment/yawamf/.runtime/docker-compose.monolith.yml',
    '-f', 'deployment/yawamf/compose.override.yaml'
)
docker network inspect birdcam_default --format '{{.Name}}'
docker @composeArgs config --quiet
docker @composeArgs pull
docker @composeArgs up -d
docker @composeArgs ps
```

The broker has **no host port**. Its name inside this network is `birdnet-mqtt`,
port 1883. Anonymous MQTT is restricted to containers on this trusted network;
use broker authentication before sharing that network with untrusted workloads.
Only YA-WAMF's administrative UI is published, at <http://127.0.0.1:9852>.
BirdNET-Go+ remains at <http://localhost:8082/ui/dashboard> (development UI:
<http://127.0.0.1:5173/ui/dashboard>).

The official container runs as Linux UID/GID 1000:1000. Named volumes retain
configuration under `/config`, models and history under `/data`, and MQTT state
under `/mosquitto/data`. They avoid Windows bind-mount ownership problems;
no Windows `sudo` or host-directory `chown` is needed. The overlay disables video
classification and clip downloads initially and limits classifier concurrency.

## Configure Frigate

Back up the existing Frigate configuration before editing it through Frigate's
configuration editor. Merge the following into the existing sections; keep all
camera inputs, object filters, recordings, go2rtc and MediaMTX settings:

```yaml
mqtt:
  enabled: true
  host: birdnet-mqtt
  port: 1883
  topic_prefix: frigate
classification:
  bird:
    enabled: false
```

Validate and restart Frigate after this change. Disable only its native **bird
species classifier**, so two classifiers do not overwrite each other's labels.
Keep bird object detection enabled and `bird` in the feeder camera's tracked
objects. Snapshots must remain enabled. YA-WAMF cannot identify a bird that the
object detector never detects or whose snapshot it cannot retrieve.

In BirdNET-Go+, **Settings > Integrations > Frigate** should stay enabled with
camera `feeder`, snapshot import enabled and its existing working URL. The Windows
backend uses `http://127.0.0.1:5000`; YA-WAMF uses the container URL from `.env`.
Keep the 20-second poll and 3600-second overlap initially. Settings hot-reload.

## Configure YA-WAMF

Open its UI and complete the initial owner-account setup on a new installation.
For this machine's already configured installation, the generated account is
stored in the protected, ignored `.runtime/admin-credentials.json`; do not paste
that file into logs or source control.

Use the setup wizard or Settings and Model Manager to save:

1. Frigate HTTP URL and MQTT broker above, topic `frigate/events`, camera **feeder
   only**. Environment connection settings take precedence over UI values.
2. **RoPE ViT-B14**, model ID `rope_vit_b14_inat21`: download, validate with
   **CPU**, then activate. A downloaded model is not necessarily the active model.
3. Enable **Write Frigate sublabel**. Disable **Trust Frigate sublabel** so this
   worker classifies images rather than trusting its own previous output.
4. Disable YA-WAMF's **BirdNET integration**. Keep existing BirdNET-Go+ audio
   analysis and correlation in BirdNET-Go+ itself.
5. Start classification threshold at **0.70**, minimum confidence **0.40**.
   Values below the write-back threshold must remain unidentified in Frigate.
   Adjust through YA-WAMF Settings after reviewing real images. The model's
   catalogue suggests 0.45, but that is not a measured accuracy guarantee for
   this feeder; 0.70 is the conservative initial deployment choice.
6. Leave automatic video classification, clip downloads and recording clips off
   during initial testing. Use one live and one background worker, CPU providers.

Camera/model choices persist in the config volume. They are deliberately not
invented environment settings. CPU is the initial choice for the local
i5-9600K/Radeon RX 7600 host; this deployment has not validated GPU acceleration.
The classifier and detector confidence are different scores.

## Start, stop, restart and logs

From the repository root, define `$composeArgs` as above in each new shell:

```powershell
docker @composeArgs up -d
docker @composeArgs stop
docker @composeArgs start
docker @composeArgs restart yawamf
docker @composeArgs logs --tail 150 yawamf mosquitto
docker @composeArgs logs --follow --tail 50 yawamf
docker logs --tail 100 birdcam-frigate-1
Invoke-RestMethod http://127.0.0.1:9852/health |
    Select-Object status, version, mqtt
```

Ctrl+C exits log following. `stop` leaves volumes and containers intact; `down`
also preserves named volumes unless `--volumes`/`-v` is supplied. Do not use those
volume-removal flags. A model load can briefly return 502 during startup; wait
for the health check rather than resetting the account or deleting data.

To inspect MQTT without exposing a host port:

```powershell
docker exec birdnet-yawamf-mosquitto-1 mosquitto_sub -h 127.0.0.1 -t frigate/available -C 1 -W 15
# During a real bird visit; do not publish fake bird events into production:
docker exec birdnet-yawamf-mosquitto-1 mosquitto_sub -h 127.0.0.1 -t frigate/events -C 1 -W 60
```

Treat event payloads and snapshots as private data when sharing diagnostics.
YA-WAMF also provides a harmless MQTT test-publish action on `yawamf/test`.

## BirdNET updates, provenance and manual corrections

The existing poller rechecks event start times in the most recent hour. A delayed
sublabel updates the same `(source, instance, Frigate event ID)` record, even after
the durable cursor has passed its start time. The snapshot and timestamps are
retained. Changes later than this bounded window require increasing the overlap
and replaying the relevant history deliberately; there is no unbounded scan.

Camera search results and dashboard cards display classification source.
`frigate_sublabel` means the label was read from Frigate; it does **not** identify
the classifier model. Frigate's write-back API does not carry YA-WAMF model ID,
version or a manual-review marker. YA-WAMF retains its own model history; historical
Frigate labels are not retroactively attributed to RoPE. Missing species scores
remain unknown, never copied from the object detector score.

An explicit BirdNET owner review locks canonical species against later automatic
updates. See [review API and PowerShell example](../../doc/FRIGATE.md#owner-species-review).
Its source becomes `manual`, `speciesVerified` becomes true and the automatic
confidence is cleared. Raw incoming sublabels and event metadata continue updating.
Manual edits made only in YA-WAMF/Frigate do not automatically create this BirdNET
lock because the upstream event has no reliable manual flag.

Audio correlations remain time/species candidates. They do not prove the
photographed individual made the matching call. Audio counts and tables are
unchanged; visual data uses the v2 repositories only, with additive schema fields.

## Troubleshooting

| Symptom                 | Check                                                                                                                                                                                                                                      |
| ----------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| No bird events          | First inspect Frigate live view, camera FPS, tracked `bird` objects and snapshots. Zero FPS means no inference input. Showing a printed bird may fail the object detector. YA-WAMF cannot fix that stage.                                  |
| MQTT disconnected       | Both containers must join `birdcam_default`; use `birdnet-mqtt`, not `localhost`. Confirm Frigate `topic_prefix: frigate`, broker health and worker logs showing `frigate/events`. No Windows port forwarding is needed.                   |
| HTTP/HTTPS error        | Use internal `http://birdcam-frigate-1:5000` from Docker. Use the host loopback URL for the native backend. Do not disable certificate verification to mix 5000 with HTTPS.                                                                |
| Bird stays unidentified | Check the actual active model, CPU validation, YA-WAMF threshold and snapshot availability. Generic bird detections are expected when no reliable species passes the threshold.                                                            |
| Wrong species           | Review the crop, lighting and object box first; raise the threshold if needed. Compare regional models on the same saved images before changing production. Make an owner correction through the review API when necessary.                |
| Delayed species missing | Wait for the next 20-second poll, check BirdNET integration status, time synchronization and one-hour overlap. Replays update one record; they should not create duplicates.                                                               |
| Model/permission error  | Use managed volumes and inspect YA-WAMF health/logs. Download, validate and activate through Model Manager. Do not delete `/data` or change Windows ownership blindly.                                                                     |
| Restart or brief outage | Containers use restart policies; MQTT clients reconnect. Check health and `frigate/available` after recovery. BirdNET retains its cursor and retries HTTP failures. This is not a guarantee of replaying MQTT events lost during downtime. |

## Compare North American bird models later

After a real RoPE baseline is collected, set **Bird model region > North America**
in YA-WAMF's setup wizard or **Settings > Detection**. Model Manager offers
**Small Birds** (NA EfficientNet-B0, about 18 MB) and **Medium Birds** (NA Binocular,
about 333 MB). Their region-specific vocabularies differ from RoPE's wildlife-wide
10,000 outputs. See the release's [model guide](https://github.com/Jellman86/YetAnother-WhosAtMyFeeder/blob/v2.23.2/docs/features/ai-models.md).

Download and CPU-validate each candidate before activation. Compare the same
representative saved feeder images with known species, including empty/poor crops
and unfamiliar birds. Record model ID/region, scores, correct/incorrect/unknown
counts, latency and RAM. Use offline analysis with Frigate write-back disabled
during comparisons; do not run two competing production writers. Restore RoPE
and write-back after comparison unless the regional candidate performs better.
Do not treat model catalogue accuracy or this host's validation latency as measured
accuracy on live birds.

## Verification record and remaining live test

On this Windows Docker Desktop deployment, official YA-WAMF 2.23.2 CPU and the
broker were started, model hashes checked by Model Manager, and RoPE validated
with ONNX Runtime CPUExecutionProvider (one validation inference: about 185 ms).
The selected model was activated; worker logs confirmed the two Frigate topic
subscriptions with BirdNET ingestion disabled. Frigate config validation passed
with MQTT enabled and native species classification disabled, while its camera,
recording and object-detection configuration were preserved.

YA-WAMF restart preserved its authenticated account, active validated RoPE model
and settings. Restarting the broker recovered both clients automatically;
YA-WAMF reported connected and Frigate again retained `online`. The classifier
container could reach Frigate's HTTP API and write both persistent directories as
UID/GID 1000:1000. These connectivity/recovery checks involved no fabricated bird
events; YA-WAMF's event history remained empty.

Automated Go tests use an isolated HTTP server and temporary SQLite repositories
in both naming modes. They cover unidentified import, delayed labels after the
cursor, changed provisional species, replay deduplication, timestamps/snapshot
retention, explicit manual locks and restart with a durable cursor. Existing tests
cover HTTP retry, unavailable snapshots and confidence normalization. API tests
exercise authentication, valid/invalid owner reviews and unlocking; frontend tests
check source rendering without altering image sizing.

The complete default Go race suite and Go lint passed in Linux/WSL. Frontend
quality checks, all translation checks, 13 targeted frontend tests and the
production build passed. The Windows executable was built, smoke-tested and
deployed locally. Browser checks opened dashboard, Live Cameras and Search on
both 8082 and 5173 without JavaScript errors. Backend configuration and audio
records were preserved. Other platform builds and MySQL/MariaDB integration
suites were not run for this deployment.

**Live bird testing is deferred at the owner's request.** Camera `feeder` reported
0 FPS during setup. Model validation and MQTT connectivity do not prove the full
bird-to-species pipeline. When the stream is available, observe one real event:
confirm its ID in `frigate/events`, YA-WAMF inference and write-back, the sublabel
and score in Frigate, then the same event with snapshot/source in BirdNET Search
Detections and dashboard. Confirm repeated polls keep one record, a species update
changes it, low-confidence birds stay unidentified, and audio continues normally.
Do not recreate the previously deleted test detections.

Implementation changes are the overlay/example/broker config in this directory,
v2 visual entity/repository review fields, API-v2 owner review endpoint, delayed
label regression tests, and source captions/translations in existing camera cards
and results. No direct YA-WAMF-to-BirdNET ingestion endpoint was added.
