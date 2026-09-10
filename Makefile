.PHONY: all generate-client test build vet tidy

# Full end-to-end: advance submodule to origin/main, re-merge upstream specs, regenerate Go client.
all:
	UPDATE_SUBMODULE=1 RUN_MERGE=1 $(MAKE) generate-client

# Regenerate client_gen_*.go from the merged OpenAPI spec.
# Set UPDATE_SUBMODULE=1 to advance submodule to origin/main first.
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
