# Development tool configuration

Run tools from the repository root. Task and CI pass these configuration paths
explicitly:

| File            | Purpose               | Command                                 |
| --------------- | --------------------- | --------------------------------------- |
| `air.toml`      | Backend reload server | `task dev_server`                       |
| `golangci.yaml` | Go linting            | `task lint`                             |
| `mockery.yaml`  | Generated test mocks  | `go generate ./internal/datastore`      |
| `cliff.toml`    | Changelog formatting  | `git-cliff --config .config/cliff.toml` |

For direct invocation, use `air -c .config/air.toml realtime`,
`golangci-lint run --config .config/golangci.yaml`, or
`mockery --config .config/mockery.yaml`.

Go module files, `main.go`, `Taskfile.yml`, `Dockerfile`, `LICENSE`, and the Git
configuration files remain at the root for standard tooling and discovery.
