#!/bin/bash

# Generates client.gen.go from the curated bill_analysis spec in the unified-openapi submodule.
#
# The unified-openapi submodule must be initialized:
#   git submodule update --init --recursive
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$SCRIPT_DIR/../.."
SPEC_FILE="$REPO_ROOT/unified-openapi/generator/sources/rightscale/bill_analysis/openapi.json"

if [ ! -f "$SPEC_FILE" ]; then
  echo "Error: $SPEC_FILE not found. Run 'git submodule update --init --recursive'." >&2
  exit 1
fi

# Generate the client code from the OpenAPI 3 spec
cd "$SCRIPT_DIR"
rm -f client.gen.go
go tool oapi-codegen -package billanalysis -config config.yaml "$SPEC_FILE"
echo "✓ client.gen.go regenerated from unified-openapi"
