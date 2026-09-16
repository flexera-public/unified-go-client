.PHONY: all update-unified-openapi update-deps generate generate-client test build vet tidy

# Resolve the requested upstream branch (main by default), download its
# immutable OpenAPI snapshot, and record the resolved commit/hash.
update-unified-openapi:
	./scripts/update-unified-openapi $(REF)

# Update all Go module dependencies to their latest versions and tidy
# go.mod/go.sum.
update-deps:
	go get -u ./...
	go mod tidy

# Full end-to-end: update Go dependencies and regenerate the client from the
# committed unified-openapi snapshot.
all: update-deps generate

# Alias for generate-client.
generate: generate-client

# Regenerate client_gen_*.go from the committed unified-openapi/openapi3.json.
generate-client:
	go generate .

test:
	go test ./...

build:
	go build ./...

vet:
	go vet ./...

tidy:
	go mod tidy
