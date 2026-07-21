#!/bin/bash



# Use centralized OpenAPI spec from openapi_specs repository
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OPENAPI_DIR="$SCRIPT_DIR/../../../openapi"
SPEC_DIR="$OPENAPI_DIR"

# Fetch latest
# (cd "$OPENAPI_DIR" && go run . fetch rightscale-optima-recommendations --openapi_specs_dir ${SPEC_DIR})

# Generate the client code from the OpenAPI 3 spec
rm client.gen.go
go tool oapi-codegen -package optimarecommendations -config config.yaml "$SPEC_DIR/sources/rightscale/optima_recommendations/openapi.yaml"

# Use sed to replace `openapi_types.File` with `interface{}` in the generated client code
sed -i 's/openapi_types.File/interface{}/g' client.gen.go

# Use sed to replace `Tags []string `json:"tags"` with 	Tags []string `json:"tags,omitempty"`
sed -i 's/Tags \[\]string `json:"tags"`/Tags []string `json:"tags,omitempty"`/g' client.gen.go
