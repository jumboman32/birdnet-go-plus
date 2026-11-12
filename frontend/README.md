# BirdNET-Go Frontend

The web UI of BirdNET-Go: a Svelte 5 single page application (runes, TypeScript, Tailwind CSS v4) built with Vite. It is a plain Svelte SPA, not SvelteKit: routing is done client-side in `src/App.svelte` and the navigation store (see [ROUTES.md](ROUTES.md)). The production build in `frontend/dist` is embedded into the Go binary (`embed.go`) and served by the Go server under `/ui/`, with assets under `/ui/assets/`. The UI talks to the backend through the `/api/v2/` endpoints; startup configuration such as the CSRF token and the security state comes from `GET /api/v2/app/config`.

The UI targets desktop and tablet only. Phone layouts are out of scope and are not tested or fixed.

## Development

Run the commands from the repository root with [Task](https://taskfile.dev). Task installs the npm dependencies on demand (`task frontend-install`).

| Command               | Purpose                                                                                       |
| --------------------- | --------------------------------------------------------------------------------------------- |
| `task frontend-watch` | Rebuild `frontend/dist` on every change (`vite build --watch`)                                |
| `task dev_server`     | Run the Go backend with hot reload through `air` (see `.config/air.toml`), with Go files only |
| `task frontend-dev`   | Start the Vite dev server with HMR on port 5173                                               |
| `task frontend-build` | Type-check and build the production bundle                                                    |
| `task dev`            | Print the development workflow options                                                        |

There are two ways to work on the UI:

1. **Watch mode, served by Go.** Run `task frontend-watch` in one terminal and `task dev_server` (or `./bin/birdnet-go realtime`) in another, then open `http://localhost:8080`. When `frontend/dist` exists on disk next to the working directory, the Go server serves the UI from disk instead of the embedded copy, so a rebuild is picked up without recompiling Go. `air` ignores the `frontend` directory.
2. **Vite dev server.** Start the isolated test backend described below, then run `task frontend-dev` and open `http://localhost:5173`. Vite proxies `/api` to `http://127.0.0.1:8081`. For a Go development backend on port 8080, set `BIRDNET_DEV_BACKEND=http://127.0.0.1:8080` before starting Vite.

Without Task, the same scripts run from `frontend/`: `npm run dev`, `npm run build`, `npm run build -- --watch`.

## Windows: start the test backend first

The frontend needs a separately running BirdNET-Go backend. `ECONNREFUSED` for
`/api/v2/app/config` means it cannot connect to that backend. Reinstalling npm
packages does not start the backend.

1. Download **Windows-Rarity-Backend** from the
   [Windows development build](https://github.com/jumboman32/birdnet-go-plus/actions/workflows/rarity-development-build.yml).
   This package supplies the backend; Vite supplies the current branch's frontend.
2. Extract the ZIP into a new folder outside your production installation. Open
   PowerShell in that extracted folder and run:

   ```powershell
   node .\start-development.mjs
   ```

   Leave this terminal open. The launcher uses port **8081**, an empty test
   database, and its own `development-data` folder. Capture is initially disabled.
   See [the launcher instructions](../scripts/development/README.md).

3. Open a second PowerShell terminal and run:

   ```powershell
   cd "$env:USERPROFILE\Documents\birdnet-go-plus\frontend"
   npm.cmd ci
   npm.cmd run dev
   ```

4. Open <http://localhost:5173/ui/dashboard>. Configure a microphone and enable an
   acoustic model in this test dashboard to collect test detections. An empty
   dashboard is expected until test detections have been collected.

The default proxy uses `127.0.0.1:8081`, avoiding the production port and Windows
IPv4/IPv6 `localhost` resolution differences. Vite does not start a backend.
For another development backend, set the address before starting Vite:

```powershell
$env:BIRDNET_DEV_BACKEND = 'http://127.0.0.1:8081'
npm.cmd run dev
```

You can also put `BIRDNET_DEV_BACKEND=http://127.0.0.1:8081` in the Git-ignored
`frontend/.env.development.local`. Restart Vite after changing the address.
This affects development proxying only; production builds use the backend
serving the UI. Use a separate test backend for verification and deletion tests,
since those actions change its data.

## Tests and checks

Run these from `frontend/` (the Task names are in brackets).

| Command                                         | Purpose                                                        |
| ----------------------------------------------- | -------------------------------------------------------------- |
| `npm test` [`task frontend-test`]               | Vitest unit and component tests                                |
| `npm run check:all` [`task frontend-lint`]      | Prettier, ESLint, Stylelint, `svelte-check` and ast-grep rules |
| `npm run typecheck` [`task frontend-typecheck`] | TypeScript and Svelte checks only                              |
| `npm run test:a11y`                             | Accessibility tests                                            |
| `npm run test:e2e`                              | Playwright end-to-end tests (see `tests/README.md`)            |
| `task frontend-quality`                         | Auto-fix, then checks, tests and build                         |

Translation checks are `npm run i18n:validate` and `npm run generate:i18n-types:check`.

## Where things live

| Path                                                                 | Contents                                                                                                                                                                                  |
| -------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `src/App.svelte`, `src/main.js`                                      | Entry points, client-side route table and page loading                                                                                                                                    |
| `src/lib/desktop/`                                                   | The tablet and desktop UI: `layouts/`, `views/` (top level pages), `features/` (analytics, dashboard, detections, import-export, live-stream, settings, system, wizard) and `components/` |
| `src/lib/components/`                                                | Shared primitives                                                                                                                                                                         |
| `src/lib/api/`, `src/lib/auth/`, `src/lib/stores/`, `src/lib/utils/` | API helpers, auth, Svelte stores and utilities                                                                                                                                            |
| `src/lib/i18n/`                                                      | Translation runtime and generated key types                                                                                                                                               |
| `src/lib/telemetry/`                                                 | Frontend error reporting                                                                                                                                                                  |
| `static/messages/`                                                   | Translation files, one JSON file per locale                                                                                                                                               |
| `tests/`                                                             | Playwright end-to-end tests                                                                                                                                                               |
| `tools/`                                                             | Screenshot tooling                                                                                                                                                                        |
| `rules/`                                                             | ast-grep rule sets used by `npm run ast:all`                                                                                                                                              |

## Guidelines

The rules for working in this directory are in [AGENTS.md](AGENTS.md). Nested guides cover the areas that have their own conventions:

- [src/lib/desktop/AGENTS.md](src/lib/desktop/AGENTS.md): the desktop and tablet UI
- [src/lib/desktop/components/AGENTS.md](src/lib/desktop/components/AGENTS.md): UI components
- [src/lib/desktop/components/ui/AGENTS.md](src/lib/desktop/components/ui/AGENTS.md): generic UI primitives
- [src/lib/desktop/features/settings/AGENTS.md](src/lib/desktop/features/settings/AGENTS.md): settings pages
- [static/messages/AGENTS.md](static/messages/AGENTS.md): translations
- [tools/AGENTS.md](tools/AGENTS.md): screenshot tooling
