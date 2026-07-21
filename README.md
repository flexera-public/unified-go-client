# unified-go-client

[![Go Reference](https://pkg.go.dev/badge/github.com/flexera-public/unified-go-client.svg)](https://pkg.go.dev/github.com/flexera-public/unified-go-client)
[![test](https://github.com/flexera-public/unified-go-client/actions/workflows/test.yml/badge.svg)](https://github.com/flexera-public/unified-go-client/actions/workflows/test.yml)

Go client for the Flexera One unified API. Generated from the [unified-openapi](https://github.com/flexera-public/unified-openapi) spec using [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen).

Requires Go 1.24.5 or newer.

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

The client code (`client_gen.go`) is generated from the Flexera One unified OpenAPI spec. To regenerate after the spec changes:

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

## License

See [LICENSE](LICENSE).
