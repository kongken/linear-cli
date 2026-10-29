# workctl

Command-line client for Linear and Plane, written in Go. The existing `linear` executable remains available with its original commands, configuration, credentials, and JSON output.

Plane support is a preview validated against local API fixtures. Authenticated Cloud and self-hosted deployment validation is still pending; see the [compatibility matrix](docs/compatibility.md).

## Build

```bash
go build -o workctl ./cmd/workctl
go build -o linear ./cmd/linear
```

The existing installation path remains supported:

```bash
go install github.com/kongken/linear-cli/cmd/linear@latest
```

Build the Plane preview from this checkout until a release containing `cmd/workctl` is published. See [installation](docs/installation.md).

## Linear

```bash
export LINEAR_API_KEY=lin_api_...
# or: linear auth login

linear issue mine
linear issue view ENG-123 --json
workctl linear issue view ENG-123 --json
```

## Plane

```bash
# Prompts for a personal access token without displaying it.
workctl plane auth login --profile cloud --workspace my-workspace

workctl plane project list --json
workctl plane --project PROJECT_UUID issue list
workctl plane issue view ENG-123 --json
workctl plane --project PROJECT_UUID issue create --title 'Fix login' --priority high
workctl plane issue comment add ENG-123 --body 'Ready for review'
```

Use `--api-version v1` with the explicit instance URL for a self-hosted v1 deployment. [Plane usage](docs/plane.md) covers authentication, work items, comments, resource management, attachments, and queries. [Configuration](docs/configuration.md) describes profile selection and credential isolation; [migration](docs/migration.md) covers existing scripts and rollback.

## Develop

```bash
go test ./...
go vet ./...
go test -race ./internal/plane/... ./internal/planeconfig ./internal/planeauth ./internal/cmdplane ./internal/workcli
go generate ./internal/gql/
go build -o linear ./cmd/linear
go build -o workctl ./cmd/workctl
```

GraphQL operations remain in `internal/gql/queries/`; regenerate after changing them. Plane uses a separate REST client with internal v1/v2 adapters and `httptest` fixtures. Regular tests do not contact Plane or use real credentials.

- [API research](docs/research/plane-api.md)
- [Implementation plan](docs/plans/plane-support.md)
- [Current capability and validation matrix](docs/compatibility.md)
