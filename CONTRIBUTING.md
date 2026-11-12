# Contributing to BirdNET-Go+

This is an independent fork of [BirdNET-Go](https://github.com/tphakala/birdnet-go).
Changes here do not require upstream feature approval or an upstream pull request.

## Development

Follow [README.md](README.md) for setup and installation. Read [AGENTS.md](AGENTS.md)
and the relevant module guide before changing code.

- Go: run `task setup-dev` once, then use `task lint` and `task test`.
- Frontend: from `frontend/`, use `npm run check:all` and the relevant `npm test` suites.
- Windows frontend development: use the [separate test backend](scripts/development/README.md).
- Documentation: format changed Markdown with Prettier from `frontend/`.

Keep normal commit hooks enabled. Report unavailable tools and incomplete checks;
do not mark checks as passing without running them. Documentation-only changes
do not need the Go or frontend application test suites.

## Submitting Changes

Keep commits focused and use Conventional Commit messages, for example
`fix(calendar): preserve the selected week` or `docs: clarify installation`.
Describe what changed and how it was verified. Mention breaking changes and
known limitations. Do not include credentials, private configuration, recordings,
or local databases.

Pushes and pull requests within this fork do not require preflight certification
or a prescribed multi-agent review. Run checks relevant to the change and retain
the existing technical and data-safety rules.

For a contribution to the original project, follow its
[upstream contribution guidelines](https://github.com/tphakala/birdnet-go/blob/main/CONTRIBUTING.md)
separately.

## License and Attribution

See [LICENSE](LICENSE) for the project license. Preserve existing copyright,
license notices, [AUTHORS](AUTHORS), and [third-party attribution](LICENSES.md).
