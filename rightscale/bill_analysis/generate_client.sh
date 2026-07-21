#!/bin/bash

# Use centralized OpenAPI spec from openapi_specs repository
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OPENAPI_DIR="$SCRIPT_DIR/../../../openapi"
SPEC_DIR="$OPENAPI_DIR"

# Fetch latest
# (cd "$OPENAPI_DIR" && go run . fetch rightscale-bill-analysis --openapi_specs_dir ${SPEC_DIR})

# Generate the client code from the OpenAPI 3 spec
rm client.gen.go
go tool oapi-codegen -package billanalysis -config config.yaml "$SPEC_DIR/sources/rightscale/bill_analysis/openapi.json"

# Replace `openapi_types.Date` with `time.Time` type
# This from misinterpretation of in the OpenAPI spec
sed -i "s|openapi_types.Date|time.Time|g" client.gen.go

# Remove openapi_types import as it's not referenced by anything anymore
sed -i "s|openapi_types \"github.com/oapi-codegen/runtime/types\"||g" client.gen.go
