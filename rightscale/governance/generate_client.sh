#!/bin/bash


# Use centralized OpenAPI spec from openapi_specs repository
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OPENAPI_DIR="$SCRIPT_DIR/../../../openapi"
SPEC_DIR="$OPENAPI_DIR"

# Fetch latest
# (cd "$OPENAPI_DIR" && go run . fetch rightscale-governance --openapi_specs_dir ${SPEC_DIR})

# Generate the client code from the OpenAPI 3 spec
go tool oapi-codegen -package governance -config config.yaml "$SPEC_DIR/sources/rightscale/governance/openapi.yaml"