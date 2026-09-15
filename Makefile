.PHONY: all update-submodule update-deps generate generate-client test build vet tidy

# Update the unified-openapi submodule to the latest commit on its configured
# upstream branch. This is intended for the automated refresh workflow.
update-submodule:
	git submodule update --init --remote --merge unified-openapi

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
