# AI Agent Instructions for BirdNET-Go+

This repository is an independent fork of BirdNET-Go. Follow the user's goals
for this fork; do not assume changes are intended for the upstream project.
These instructions and the module guides below apply to coding agents and
contributors.

## Module Guides

Guidance is split by area. Tools that support nested `AGENTS.md` files load the
matching file when you work in that directory; if yours does not, read it
yourself before touching code in that area.

| Working on                                       | Read first                                                                          |
| ------------------------------------------------ | ----------------------------------------------------------------------------------- |
| Any Go code, including `cmd/` and `main.go`      | `internal/AGENTS.md` (nested loading does not reach it from `cmd/` or the root)     |
| API v2 endpoints (`internal/api/v2/`)            | `internal/api/v2/AGENTS.md`, which sends you to its `README.md` endpoint catalog    |
| Error handling (`internal/errors/`)              | `internal/errors/AGENTS.md`                                                         |
| Database and persistence (`internal/datastore/`) | `internal/datastore/AGENTS.md` (the legacy database is deprecated, never expand it) |
| Frontend (Svelte 5, TypeScript)                  | `frontend/AGENTS.md`                                                                |
| Desktop UI (tablet and desktop only)             | `frontend/src/lib/desktop/AGENTS.md`                                                |
| Desktop UI components                            | `frontend/src/lib/desktop/components/AGENTS.md`                                     |
| Generic UI primitives                            | `frontend/src/lib/desktop/components/ui/AGENTS.md`                                  |
| Settings pages                                   | `frontend/src/lib/desktop/features/settings/AGENTS.md`                              |
| Translations (`frontend/static/messages/`)       | `frontend/static/messages/AGENTS.md`                                                |
| Screenshot tooling                               | `frontend/tools/AGENTS.md`                                                          |
| Writing or changing Go tests                     | The Testing section of `internal/AGENTS.md` (all Go tests MUST use testify)         |
| Writing or changing frontend tests               | the Testing section of `frontend/AGENTS.md`                                         |

## Project Context

- **Backend**: Go 1.27 (see `go.mod`), Echo HTTP server, GORM datastore
- **Frontend**: Svelte 5 (runes), TypeScript, Tailwind CSS v4, Vite, Vitest
- **Build system**: [Task](https://taskfile.dev) (`Taskfile.yml`)
- **Linting**: `golangci-lint` (Go), `npm run check:all` (frontend)
- **Review**: repository CI and optional automated reviews

| Path         | Purpose                        |
| ------------ | ------------------------------ |
| `/cmd/`      | CLI commands (Cobra/Viper)     |
| `/internal/` | Private Go packages            |
| `/rules/`    | Custom go-ruleguard lint rules |
| `/frontend/` | Svelte 5 web UI                |

## Critical Constraints

- **API v1 is frozen.** Never add or extend v1 endpoints; all new endpoints go
  in `internal/api/v2/`.
- **HARD RULE: the legacy database is deprecated and is going away. Never
  expand it.** The pre-v2 database (the `notes` schema and its GORM models, the
  legacy `DataStore`, `SQLiteStore` and `MySQLStore` in `internal/datastore`, the
  legacy and dual-write modes, and the `datastore.Interface` API shaped around
  them) will be removed completely. A PR must not add to it: no new columns,
  tables, models, fields, methods, optional interfaces, mock methods, dual-write
  paths or new callers of `c.DS` or `datastore.Interface` for persistence, and no
  shims, adapters or fallbacks for it. Write persistence code for the v2 schema
  only and go through the repository interfaces (`datastore.DetectionRepository`
  and the v2 repositories). Bug fixes that keep behavior, code that removes
  legacy code or moves callers to the repositories, and the legacy to v2
  migration code are allowed. If a change seems to need the legacy store, stop
  and ask the maintainer. Details: `internal/datastore/AGENTS.md`.
- **Reconcile database problems safely; never migrate data on your own.** When
  startup or runtime code finds a recoverable database problem, it repairs it
  automatically as long as no row of user data is lost or hidden and the repair
  is logged (two regenerable cache tables are not user data; the exception and
  its conditions are in `internal/datastore/AGENTS.md`). A data migration (SQLite to MySQL, legacy to v2, copying or
  consolidating databases) never starts by itself; only an explicit user action
  starts it. Details: `internal/datastore/AGENTS.md`.
- **HARD RULE: support only the MySQL and MariaDB LTS releases we test.** A
  tested release is supported; a release we do not test is not, and a release
  already end of life upstream is not tested (MySQL 5.7 and 8.0, MariaDB 10.6
  and older). No workarounds, version branches or tests for unsupported
  releases, no fixes for bugs that only show on them, and no end of life dates
  in code. The current list is in `internal/datastore/AGENTS.md`.
- **The frontend is for desktop and tablet only; mobile is out of scope.** Do
  not optimize, fix, test or file issues for phone viewports or phone layouts
  (about 640 px wide and below), and do not add phone-specific workarounds.
  Mobile will get its own, separate UI. A defect that shows only at phone width
  is not a defect. Tablets (touch, and narrow desktop windows at the `sm:` and
  `md:` breakpoints) still count. This is a standing maintainer decision, not
  a per-task call.
- **Settings must hot-reload.** Every setting changed through the UI must take
  effect immediately, without a server restart. Read settings per request or
  per operation (for example dynamic middleware or an atomic settings snapshot);
  never branch once on a value captured at startup.
- **No magic numbers or strings.** Use named constants with descriptive names.
- **Use `internal/errors`, never the standard `errors` package**, in Go code.
  It passes through the standard functions under their own names, except the
  standard `errors.New`, which is `errors.NewStd` there (its `errors.New` wraps
  an existing error; see `internal/errors/AGENTS.md`). `depguard` rejects the
  standard import.
- **Ultrasonic (bat) captures are always lossless.** Captures above 48 kHz are
  exported only as WAV or FLAC (`Export.UltrasonicType`, default FLAC), never as
  AAC, MP3 or Opus: lossy encoders cannot carry ultrasonic content. Do not add
  lossy handling, fallbacks or defensive branches for it.
- **Never log secrets or PII.** Use the typed sensitive-field helpers in
  `internal/logger` (a `forbidigo` lint rule enforces this for credential and
  token field names).
- **Document all exported symbols.**
- **Respect the working branch.** Continue ongoing work on its current branch.
  For new independent work, branch from this fork's current main when practical.
  Do not merge upstream changes unless requested.
- **Format the Markdown you change with Prettier**: from `frontend/`, run
  `npx prettier --write ../<path to each changed .md file>`. Skip files under
  `.agents/skills/`, which are not Prettier-formatted. Do not run
  `task format-md` in a feature PR: it reformats every Markdown file in the
  repository.

## Changes and Verification

Keep changes focused on the user's request and preserve unrelated work. Use
Conventional Commit messages. A push to this fork does not require upstream
feature approval, a mandatory multi-agent review, or a preflight certification.
Do not create a PR or merge a branch unless requested.

Run checks appropriate to the files changed:

- Go changes: run `task lint` and `task test`.
- Frontend changes: run `npm run check:all` and relevant tests from `frontend/`.
- Documentation changes: format the changed Markdown and check local links.
- Workflow changes: validate the changed YAML and referenced scripts.

Keep the normal commit hooks enabled. Report missing tools, failed checks, or
checks that were not run; never claim an unobserved pass. If the user explicitly
authorizes a push with incomplete checks, report the limitation and proceed.
Documentation-only changes do not require running the application test suites.

Run `task setup-dev` before Go checks: it installs the TensorFlow Lite headers
and C library. Task supplies the build tags and CGO flags. Use `task --dry lint`
or `task --dry test` to inspect the commands rather than copying stale flags.

Default checks cover only the current OS and build tags. For tagged code, use
`task lint BASE_BUILD_TAGS=<tags>`; for OpenVINO, first run
`task check-openvino OPENVINO=true`. Use CI for other platforms and report
platforms that were not verified. Treat cross-platform build failures as real
failures.

## Pull Requests

When requested, describe the concrete change, validation results, and any
breaking behavior using the short template in `.github/pull_request_template.md`.
Keep human declarations for the author to complete. Upstream's contribution
process applies only when the user explicitly requests an upstream PR.

## Interpreting CI Failures

The `golangci-test` workflow runs Go tests through `gotestsum` with one
automatic rerun of any failed test, then publishes a consolidated result in the
`test-report` job. Before assuming a red run means your code is broken:

1. Read the `test-report` job summary. It states one verdict:
   - `REGRESSION`: real failures that persisted after a rerun. Fix these.
   - `PASS (with flakes)`: tests that failed once then passed on rerun. These
     are flaky/infra (a reaped container, a registry blip), NOT a code
     regression. Do not "fix" them; re-run or report instead.
   - `PASS`: all green.
2. For machine-readable detail, download the `ci-failures` artifact:
   - `ci-failures.json`: array of real regressions (`{run, pkg, test, output}`).
   - `ci-flaky.json`: tests that passed on rerun (`{run, pkg, test}`, informational).

   `run` names the test run (the event file without its `events-` prefix and
   `.json` suffix). A test that fails in one run and passes in another is a
   regression in the first, not a flake; only a fail followed by a pass inside
   one run counts as flaky.

   Prefer these small files over scrolling multi-thousand-line raw logs.

3. If a testcontainer job failed, the job summary includes a "Testcontainer
   diagnostics" block (docker state, memory, OOM kills) to tell an infra flake
   from a logic bug.

Do not spend time debugging a failure classified as flaky/infra. If a test is
persistently flaky, raise it rather than patching around it.

## Build Commands

| Command                   | Purpose                                                                    |
| ------------------------- | -------------------------------------------------------------------------- |
| `task`                    | Default build (auto-detects the target)                                    |
| `task dev_server`         | Backend development server with reload                                     |
| `task frontend-build`     | Frontend production build                                                  |
| `task test` / `task lint` | Go tests / Go lint                                                         |
| `task frontend-quality`   | Frontend auto-fix (format, lint, ast-grep), then checks, tests, and build  |
| `task clean`              | Remove build artifacts                                                     |
| `task linux_amd64`        | Cross-platform build (also `linux_arm64`, `windows_amd64`, `darwin_arm64`) |

Run `task --list` for everything else (setup, integration tests, native library downloads).

## Code Search and Refactoring

Prefer syntax-aware tools over plain text search for code operations. Use
`ast-grep` (`sg`) for structural searches and rewrites instead of `grep`/`sed`
regexes, which break on formatting and match inside strings and comments:

```bash
# Structural search
ast-grep --pattern 'console.$METHOD($$$)' frontend/src/

# Syntax-safe rewrite: prints the diff only; add --update-all (-U) to apply it
ast-grep --pattern 'let $VAR = $VALUE' --rewrite 'const $VAR = $VALUE' frontend/src/
```

If your tool offers language-server navigation (find references, go to
definition), use it for "who calls this" questions before changing a signature.
See `frontend/doc/AST-GREP-SETUP.md` for the frontend ast-grep rules.

## Handling Review Feedback

Verify review suggestions before implementing them. Explain incorrect or
out-of-scope suggestions, and clarify ambiguous behavior changes with the user.

## About These Instruction Files

Keep cross-cutting guidance in this file and area-specific guidance in the
matching module's `AGENTS.md`. Keep instructions focused and remove stale
rules. Do not add parallel `CLAUDE.md`, `CLAUDE.local.md`, or `GEMINI.md`
files. Store personal machine-specific instructions in gitignored local files.
