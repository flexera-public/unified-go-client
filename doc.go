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
//	The generator reads unified-openapi/openapi3.json as committed. The
//	unified-openapi/PIN file records the upstream revision used for the snapshot.
//
// # Sub-packages
//
// Governance is not yet included in the merged OpenAPI artifact and remains
// available as a standalone client:
//
//	github.com/flexera-public/unified-go-client/rightscale/governance
//
// Bill Analysis and other Optima operations are generated from the merged
// upstream artifact in the root package; use BillAnalysis*, BillingCenterService*,
// and OptimaRecommendations* methods on *ClientWithResponses.
//
// Anomaly investigation workflow:
//
//	github.com/flexera-public/unified-go-client/anomaly
package flexera
