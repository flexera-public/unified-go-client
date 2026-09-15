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
// To regenerate the client from the committed unified-openapi snapshot run:
//
//	make generate-client
//
//	The generator reads unified-openapi/openapi3.json as committed. Updating that
//	spec is a separate reviewed change in the unified-openapi repository.
//
// # Sub-packages
//
// Legacy per-service RightScale/Optima clients (used via service/optima bundle):
//
//	github.com/flexera-public/unified-go-client/rightscale/bill_analysis
//	github.com/flexera-public/unified-go-client/rightscale/billing_center_service
//	... (see rightscale/)
//
// The rightscale/bill_analysis package is generated from a hand-curated minimal
// OpenAPI subset in unified-openapi, covering only Bill Analysis endpoints not yet
// migrated to Flexera API Gateway. It exposes backward-compatible type aliases
// (compat.go) so existing callers require no import-path changes. New code should
// prefer the root BillAnalysis* methods on *ClientWithResponses directly.
//
// Anomaly investigation workflow:
//
//	github.com/flexera-public/unified-go-client/anomaly
package flexera
