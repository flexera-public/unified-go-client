package finopscustomizations

//go:generate sh -c "cd ../../../../openapi && go run . fetch flexera-finops-customizations-v1"
//go:generate go tool oapi-codegen -config config.yaml -package finopscustomizations ../../../../openapi/sources/flexera/finops_customizations/v1/openapi.json
