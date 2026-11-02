# Isolated Windows development backend

This launcher is packaged beside `birdnet-go-dev.exe`, `tensorflowlite_c.dll`,
`onnxruntime.dll`, and `onnxruntime_providers_shared.dll`. The executable includes
the frontend; Vite and Go are not needed to run this package. Node.js is required.
If Windows reports a missing `VCRUNTIME140` or `MSVCP140` DLL, install Microsoft's
[Visual C++ x64 Redistributable](https://aka.ms/vs/17/release/vc_redist.x64.exe).

1. Extract the entire ZIP into a **new directory**, outside your production install.
2. Double-click `Start-Development.cmd`, or run `node start-development.mjs` there.
3. Leave the terminal open and visit <http://localhost:8081/ui/dashboard>.
4. In this development UI, open Settings → Integrations, enable eBird and enter a
   replacement key privately. A key posted in chat should be revoked and replaced.
5. Configure the microphone and enable an acoustic model in this development UI
   if you want new test detections. The initial database is empty and capture is off.

All initial configuration, SQLite data, model downloads, logs and recordings live
under the extracted directory's `development-data`. Existing development settings
are retained on subsequent launches. Weather, uploads and telemetry are initially
disabled. Port 8081 is reserved for this backend; the launcher stops if occupied.
Stop it with Ctrl+C. Your port-8080 installation is separate.

## Testing rarity

The real endpoint can be opened directly, even before recording a detection:

<http://localhost:8081/api/v2/species/local-rarity?scientific_name=Cyanocitta%20cristata&date=2026-11-02>

Change `date` to your observation date and `scientific_name` to the species.
Without historical data, expect `score: null` and `reasonCode: "missing_data"`.
An eBird key enables recent supporting observations; it cannot supply the
multi-year complete-checklist sample needed for a numeric score.

Obtain an authorized all-species eBird Basic Dataset and matching Sampling Event
Data, then use the repository's `scripts/import-local-rarity.py` as described in
[the rarity documentation](https://github.com/jumboman32/birdnet-go-plus/blob/feature/local-rarity-score/docs/local-rarity.md).
Before launching the backend, set the generated file's path in PowerShell:

```powershell
$env:BIRDNET_LOCAL_RARITY_DATA = 'C:\BirdData\local-rarity.json'
node .\start-development.mjs
```

The initial station is 39.37, -74.57 near Linwood, NJ. Use the same station
coordinates for the import and development settings. A numeric result requires
at least 100 eligible locality-days across three prior years in the seasonal
window. This package contains no historical dataset and no API key.

The development build was cross-compiled for Windows x64. Backend startup and
numeric scoring were checked on Linux; native Windows execution still needs your
test. A separate synthetic frontend preview remains available from the repository
with `npm.cmd run dev:rarity`; that preview does not test this backend.
