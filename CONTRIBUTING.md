# Contributing

Install the Go version from `go.mod`. Run:

```bash
gofmt -w cmd internal
go mod tidy
go test -race ./...
go vet ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
CAPTAIN_INTEGRATION=1 go test -race -tags=integration -timeout=5m ./...
```

Integration tests require Docker Engine/Compose and create uniquely named temporary
containers, a broker and a volume. They remove only their own test resources. They
never run `docker system prune` or interact with existing projects.

Keep adapters at the boundary, business validation in the domain/application layer,
and test failure/restart behavior as carefully as the successful path. Do not add
unneeded frameworks. Document wire, state or deployment behavior changes and include
migration instructions. Add unresolved findings to `docs/problems-and-ideas.md`.

For release maintenance, run `bash scripts/release.sh X.Y.Z`, inspect the archives in
`dist/X.Y.Z/`, then create a `vX.Y.Z` tag on a reviewed, clean commit on main. The release
workflow repeats CI, builds six archives, attests them and creates a **draft** GitHub
release. Review the notes and assets before publishing. Do not replace existing version
tags/assets.
