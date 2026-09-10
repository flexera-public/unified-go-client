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
declarations.

To regenerate after the spec changes:

```sh
# Initialize the public submodule if needed:
git submodule update --init --recursive

# Regenerate from the pinned submodule commit:
make generate-client

# To also re-merge specs from upstream sources first:
RUN_MERGE=1 make generate-client

# To advance the submodule to the latest upstream main branch first:
UPDATE_SUBMODULE=1 make generate-client
```

By default, regeneration uses the `unified-openapi` commit pinned in this repository so builds are reproducible.

Generation first runs `oapi-codegen` once to preserve its normal package-wide
behavior, then `cmd/split-client` partitions the resulting Go syntax tree.
Running generation twice with the same inputs must produce no changes.

## License

See [LICENSE](LICENSE).
