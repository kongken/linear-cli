## basics

- this is a Go app (`cmd/linear`, `internal/...`)
- after editing GraphQL documents under `internal/gql/queries/`, run `go generate ./internal/gql/`
- `graphql/schema.graphql` is Linear's API schema
- for diagnostics use `go test ./...` and `go vet ./...`
- prefer `foo == nil` / `foo != nil` for pointers
- avoid `any` when a concrete type is available
- for `--json` output, preserve GraphQL field names and nesting
- for paginated `--json` output, preserve connection shape and concatenate `nodes`

## error handling

- never fail silently
- use custom error types from `internal/errors`
- wrap command actions with `errors.HandleError(err, "Failed to <action>")`

## tests

- command tests live next to code under `internal/cmd*/`
- use `NO_COLOR=1` when asserting terminal output
