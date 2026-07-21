#!/bin/bash

# Use centralized OpenAPI spec from openapi_specs repository
# The spec is pre-processed with AppliedPolicyStatus -> AppliedPolicyStatusDetail replacement
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OPENAPI_DIR="$SCRIPT_DIR/../../../../openapi"
SPEC_DIR="$OPENAPI_DIR"

# Fetch latest
# (cd "$OPENAPI_DIR" && go run . fetch flexera-policy-v1 --openapi_specs_dir ${SPEC_DIR})

# Generate the client code from the OpenAPI 3 spec
rm client.gen.go
go tool oapi-codegen -config config.yaml -package policy "$SPEC_DIR/sources/flexera/policy/v1/openapi.json"

# Replace `openapi_types.File` with `any` type
# This from misinterpretation of in the OpenAPI spec
# The "value" field a param for an applied policy can be many types, string, number, or list of strings.
sed -i "s|openapi_types.File|any|g" client.gen.go
