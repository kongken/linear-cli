# linear-cli

Command-line client for [Linear](https://linear.app), written in Go.

## Install

```bash
go install github.com/kongken/linear-cli/cmd/linear@latest
```

Or build from source:

```bash
go build -o linear ./cmd/linear
```

## Auth

```bash
export LINEAR_API_KEY=lin_api_...
# or
linear auth login
```

## Usage

```bash
linear --help
linear issue mine
linear issue view ENG-123
```

## Develop

```bash
go test ./...
go generate ./internal/gql/
go build -o linear ./cmd/linear
```

GraphQL operations live in `internal/gql/queries/`; regenerate with `go generate ./internal/gql/`.
