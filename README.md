# unified-go-client

[![Go Reference](https://pkg.go.dev/badge/github.com/flexera-public/unified-go-client.svg)](https://pkg.go.dev/github.com/flexera-public/unified-go-client)
[![test](https://github.com/flexera-public/unified-go-client/actions/workflows/test.yml/badge.svg)](https://github.com/flexera-public/unified-go-client/actions/workflows/test.yml)

Go client for the Flexera One unified API. Generated from the [unified-openapi](https://github.com/flexera-public/unified-openapi) spec using [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen).

## Experimental Project

This project is currently considered **experimental**.

While we intend to minimize disruption, breaking changes may occur as we continue to evolve the design, APIs, and implementation. **Until the project reaches a stable v1.0.0 release, backward compatibility is not guaranteed.**

We welcome feedback and contributions, but recommend evaluating the current level of stability before adopting this project in production environments.

## Import

```go
import flexera "github.com/flexera-public/unified-go-client"
```

## Quick start

```go
import (
    "os"

    flexera "github.com/flexera-public/unified-go-client"
)

helper, err := flexera.NewAuthHelper(flexera.AuthHelperConfig{Zone: flexera.ZoneNAM})
if err != nil {
    panic(err)
}

client, err := helper.NewOAuthClientWithResponses(
    flexera.OAuthConfig{RefreshToken: os.Getenv("FLEXERA_NAM_REFRESH_TOKEN")},
)
if err != nil {
    panic(err)
}

_ = client
```

## Authentication helpers

The client includes helpers for common authentication patterns:

- static bearer tokens via `WithBearerToken`
- OAuth refresh tokens via `OAuth2Doer`
- shared token refresh across multiple clients via `SharedTokenSource`
- higher-level zone-aware helpers via `AuthHelper`

Common environment variables used in examples:

- `FLEXERA_NAM_REFRESH_TOKEN`
- `FLEXERA_CLIENT_ID`
- `FLEXERA_CLIENT_SECRET`

## Development

Run the standard validation checks:

```sh
make vet
make build
make test
```

## Releasing

Tag the commit and push:

```sh
git tag v0.1.0
git push origin v0.1.0
```

The GitHub Actions release workflow will run tests and create the GitHub Release. The Go module proxy at `proxy.golang.org` will index the module automatically once the tag is publicly accessible.

## Regenerating the client

The client code (`client_gen_*.go`) is generated from the Flexera One unified
OpenAPI spec. Generated declarations are split by source specification and then
by concern (`models`, `operations`, and `responses`), while shared client
infrastructure remains in `client_gen_core.go`. The generated
`client_gen_manifest.json` maps each file back to its source spec and exported
declarations. `client_extensions_manifest.json` provides the equivalent
inventory for hand-written root-package declarations. See
[`CONTRIBUTING.md`](CONTRIBUTING.md) for the generated-vs-hand-written file
contract these manifests depend on.

The client uses a tracked, immutable `unified-openapi/openapi3.json` snapshot.
Its `PIN` file records the upstream `main` commit and the snapshot SHA-256;
the adjacent `specs.yaml` preserves the namespace metadata needed for client
generation.

To regenerate after the snapshot changes:

```sh
# Regenerate from the committed unified-openapi/openapi3.json:
make generate-client
```

Regeneration does not fetch upstream specs. To update the pinned snapshot to
the current upstream `main`, run `make update-unified-openapi`; use
`make update-unified-openapi REF=<branch>` to review another upstream branch.
The target resolves the branch to a commit before downloading, so every
committed snapshot remains reproducible.

To update Go dependencies and regenerate the client together:

```sh
make all
```

The scheduled/manual `refresh-client` workflow first pins the latest upstream
`main` snapshot, then proposes the snapshot, dependency, and generated-client
changes in a pull request.

Generation first runs `oapi-codegen` once to preserve its normal package-wide
behavior, then `cmd/split-client` partitions the resulting Go syntax tree.
Running generation twice with the same inputs must produce no changes.

## Generated vs. hand-written files

This repo mixes generated client code with hand-written helpers, in the root
package and in each `service/*`/`rightscale/*` sub-package. The distinction
is load-bearing, not cosmetic: downstream tooling (e.g. `flexera-cli`'s
`make check-client-coverage`) classifies every exported declaration as
"generated" or "hand-written" purely by checking for the standard header, in
order to flag hand-written additions (new CLI-relevant capabilities) that
don't show up in `unified-openapi/openapi3.json` and so can't be caught by
spec-diffing alone.

This is a **contract**, not an accident of `oapi-codegen`'s defaults:

- Every generated file, in the root package and in every `service/*`/
  `rightscale/*` sub-package, MUST start with the standard
  `// Code generated ... DO NOT EDIT.` header (oapi-codegen emits this by
  default; preserve it if you ever hand-edit a generator template or
  post-processing step).
- Every hand-written file MUST NOT have that header, even if it lives
  alongside generated files in the same package (e.g. `oapi-codegen.go`'s
  `//go:generate` stub, `client_extensions.go`, `compat.go`).
- In the root package, generated files are also named `client_gen_*.go` and
  are inventoried by the generator-maintained `client_gen_manifest.json`.
  Hand-written root-package files are inventoried by
  `client_extensions_manifest.json` (see below); both are regenerated by
  `make generate-client` and must be committed alongside any change to
  root-package `.go` files.
- `generated_header_test.go` enforces the naming/header pairing above across
  the whole repo (root and all sub-packages) — it fails if any generated
  file is missing the header or any hand-written file has one.

Whenever you add, rename, or remove an exported root-package declaration
in a hand-written file, run `make generate-client` and commit the resulting
diff to `client_extensions_manifest.json` along with your change.

## License

See [LICENSE](LICENSE).
