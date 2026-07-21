#!/bin/bash

# Use centralized OpenAPI spec from openapi_specs repository
# The spec is pre-converted from Swagger 2.0 to OpenAPI 3.0
export SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SPEC_DIR="$SCRIPT_DIR/../../../openapi/sources/rightscale/billing_center_service"

# Generate the client code from the OpenAPI 3 spec
go tool oapi-codegen -package billingcenterservice -config config.yaml "$SPEC_DIR/openapi.yaml"

# Fix the path in the generated client from /bcs/ to /analytics/
sed -i "s/\/bcs\//\/analytics\//g" client.gen.go