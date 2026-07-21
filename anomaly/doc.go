// Package anomaly implements the FinOps cost-anomaly investigation
// curated tool. It composes Flexera's bill_analysis anomaly-detection
// API with a billing-center resolver, AI-model → Bollinger fallback,
// dynamic threshold calibration, and an optional "discovery" mode that
// progressively relaxes constraints when no anomalies meet the primary
// recency + threshold filter.
//
// This is a port of the original implementation in
// cli/go-flexera-mcp/server/curated_cost_anomaly.go, restructured around
// the [curated.Tool] contract so the same workflow can be invoked from
// the CLI (flexera-cli curated anomaly-investigation), the MCP server,
// or any library consumer.
package anomaly
