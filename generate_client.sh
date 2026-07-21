#!/usr/bin/env bash
# Regenerate client_gen.go from the unified OpenAPI spec.
# Run from the repo root (called via `go generate .` or `make generate-client`).
#
# Prerequisites:
#   - The unified-openapi repo (github.com/flexera-public/unified-openapi) is a submodule and must be initialized and updated (run `git submodule update --init --recursive` if it isn't)
#   - openapi3.json must exist at unified-openapi/openapi3.json
#     (run `make regen` in unified-openapi if it doesn't)
#
# To also regenerate the spec from upstream sources before generating the client,
# set RUN_MERGE=1:
#
#   RUN_MERGE=1 ./generate_client.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SPEC_FILE="$SCRIPT_DIR/unified-openapi/openapi3.json"

if [ ! -d "$SCRIPT_DIR/unified-openapi" ]; then
  # Activate/Init the git submodule
  echo "→ Initializing unified-openapi submodule..."
  git submodule update --init --recursive unified-openapi
fi

# Ensure the submodule exists at the checked-in commit.
echo "→ Ensuring unified-openapi submodule is initialized at the pinned commit..."
git submodule update --init --recursive unified-openapi

# Optionally advance the submodule to the latest main branch tip.
if [ "${UPDATE_SUBMODULE:-}" = "1" ]; then
  echo "→ Updating unified-openapi submodule to origin/main..."
  (cd "$SCRIPT_DIR/unified-openapi" && git fetch origin main && git reset --hard origin/main)
fi

# Optionally re-merge upstream specs first.
if [ "${RUN_MERGE:-}" = "1" ]; then
  echo "→ Merging OpenAPI specs in unified-openapi..."
  (cd "$SCRIPT_DIR/unified-openapi/generator" && go run . merge --output-dir ..)
fi

if [ ! -f "$SPEC_FILE" ]; then
  echo "Error: $SPEC_FILE not found. Run 'make regen' in unified-openapi first." >&2
  exit 1
fi

echo "→ Generating Go client from $SPEC_FILE"
rm -f "$SCRIPT_DIR/client_gen.go"
go tool oapi-codegen -config "$SCRIPT_DIR/config.yaml" "$SPEC_FILE"

echo "✓ client_gen.go regenerated"
