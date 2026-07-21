.PHONY: generate-client test build vet tidy

# Regenerate client_gen.go from the merged OpenAPI spec.
# Assumes ../unified-openapi/openapi3.json exists.
# Set RUN_MERGE=1 to also re-merge specs from upstream sources first.
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
