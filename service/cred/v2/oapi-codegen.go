package cred

//go:generate sh -c "cd ../../../../openapi && go run . fetch flexera-cred-v2"
//go:generate go tool oapi-codegen -config config.yaml -package cred ../../../../openapi/sources/flexera/cred/v2/openapi.json
