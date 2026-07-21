package finopsonboarding

//go:generate sh -c "cd ../../../../openapi && go run . fetch flexera-finops-onboarding-v1"
//go:generate go tool oapi-codegen -config config.yaml -package finopsonboarding ../../../../openapi/sources/flexera/finops_onboarding/v1/openapi.json
