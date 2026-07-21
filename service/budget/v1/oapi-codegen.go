package budget

//go:generate echo "Generating Flexera Budget v1 client..."
//go:generate sh -c "cd ../../../../openapi && go run . fetch flexera-budget-v1"
//go:generate go tool oapi-codegen -config config.yaml -package budget ../../../../openapi/sources/flexera/budget/v1/openapi.json
