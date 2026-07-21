#!/bin/bash

# Use centralized OpenAPI spec from openapi_specs repository
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OPENAPI_DIR="$SCRIPT_DIR/../../../../openapi"
SPEC_DIR="$OPENAPI_DIR"

# Fetch latest
# (cd "$OPENAPI_DIR" && go run . fetch flexera-graphql-v1 --openapi_specs_dir ${SPEC_DIR})

# Generate the client code from the OpenAPI 3 spec
rm client.gen.go
go tool oapi-codegen -config config.yaml -package graphql "$SPEC_DIR/sources/flexera/graphql/v1/openapi.json"

# Replace `openapi_types.File` with `json.RawMessage` type
# This from misinterpretation of in the OpenAPI spec
# The "value" field a param for an applied policy can be many types, string, number, or list of strings.
sed -i "s|openapi_types.File|json.RawMessage|g" client.gen.go

# Remove openapi_types import as it's not referenced by anything anymore
sed -i "s|openapi_types \"github.com/oapi-codegen/runtime/types\"||g" client.gen.go
