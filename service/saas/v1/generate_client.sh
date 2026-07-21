#!/bin/bash

# Use centralized OpenAPI spec from openapi_specs repository
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OPENAPI_DIR="$SCRIPT_DIR/../../../../openapi"
SPEC_DIR="$OPENAPI_DIR"

# Fetch latest
# (cd "$OPENAPI_DIR" && go run . fetch flexera-saas-v1 --openapi_specs_dir ${SPEC_DIR})

# Generate the SaaS client code from the OpenAPI spec
go tool oapi-codegen -config config.yaml -package saas "$SPEC_DIR/sources/flexera/saas/v1/openapi.json"

# Replace openapi_types.File with json.RawMessage in the generated code to avoid marshaling issues
sed -i "s|openapi_types.File|json.RawMessage|g" client.gen.go