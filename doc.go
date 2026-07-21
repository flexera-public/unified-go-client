// Package flexera provides the unified Flexera One API client and helper
// packages. This is the primary import for consumers of the public SDK.
//
// # Quick start
//
//	package main
//
//	import (
//		"os"
//
//		flexera "github.com/flexera-public/unified-go-client"
//	)
//
//	func main() {
//		helper, _ := flexera.NewAuthHelper(flexera.AuthHelperConfig{Zone: flexera.ZoneNAM})
//		_, _ = helper.NewOAuthClientWithResponses(
//			flexera.OAuthConfig{RefreshToken: os.Getenv("FLEXERA_NAM_REFRESH_TOKEN")},
//		)
//	}
//
// # Regenerating the client
//
// After changing upstream API specs run:
//
//	make generate-client
//	RUN_MERGE=1 make generate-client
//	UPDATE_SUBMODULE=1 make generate-client
//
//	This merges the raw specs via the generator in the unified-openapi sibling repo,
//	then re-runs oapi-codegen to update client_gen.go.
//
// # Sub-packages
//
// Legacy per-service Flexera clients (superseded by the unified client above):
//
//	github.com/flexera-public/unified-go-client/service/budget/v1
//	github.com/flexera-public/unified-go-client/service/policy/v1
//	... (see service/)
//
// RightScale API clients:
//
//	github.com/flexera-public/unified-go-client/rightscale/bill_analysis
//	github.com/flexera-public/unified-go-client/rightscale/billing_center_service
//	... (see rightscale/)
//
// Anomaly investigation workflow:
//
//	github.com/flexera-public/unified-go-client/anomaly
package flexera
