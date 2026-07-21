package anomaly

import (
	billanalysis "github.com/flexera-public/unified-go-client/rightscale/bill_analysis"
)

func strPtr(s string) *string { return &s }

// getDimensionConfigs returns the multi-dimensional analysis matrix
// driving the investigator. Each entry produces an independent
// anomaly-detection API call.
func getDimensionConfigs() []dimensionConfig {
	excludeMarketplace := billanalysis.FilterV1{
		Type: billanalysis.FilterV1TypeNot,
		Expression: &billanalysis.FilterV1{
			Type:      billanalysis.FilterV1TypeSubstring,
			Dimension: strPtr("bill_entity"),
			Substring: strPtr("Marketplace"),
		},
	}
	excludeRegionNone := billanalysis.FilterV1{
		Type: billanalysis.FilterV1TypeNot,
		Expression: &billanalysis.FilterV1{
			Type:      billanalysis.FilterV1TypeEqual,
			Dimension: strPtr("region"),
			Value:     strPtr("None"),
		},
	}
	includeCompute := billanalysis.FilterV1{
		Type:      billanalysis.FilterV1TypeEqual,
		Dimension: strPtr("category"),
		Value:     strPtr("Compute"),
	}

	return []dimensionConfig{
		{
			Dimensions: []string{"service"},
			Filter: &billanalysis.FilterV1{
				Type:      billanalysis.FilterV1TypeEqual,
				Dimension: strPtr("capability"),
				Value:     strPtr("csm"),
			},
			Category:      "Service Anomaly",
			Description:   "Unusual spend spike in a specific cloud service",
			FinOpsContext: "Service-level anomalies often indicate misconfiguration, forgotten resources, or architectural changes.",
		},
		{
			Filter:        &excludeMarketplace,
			Dimensions:    []string{"vendor_account_name", "service"},
			Category:      "Metered Service Usage",
			Description:   "Cloud services and product usage patterns",
			FinOpsContext: "Sudden changes in service costs often indicate new deployments, scaling events, or usage pattern changes.",
		},
		{
			Filter: &billanalysis.FilterV1{
				Type: billanalysis.FilterV1TypeAnd,
				Expressions: &[]billanalysis.FilterV1{
					excludeMarketplace,
					excludeRegionNone,
				},
			},
			Dimensions:    []string{"vendor", "region"},
			Category:      "Regional Distribution",
			Description:   "Cost distribution across cloud regions",
			FinOpsContext: "Regional cost shifts may indicate geo-expansion, or unplanned multi-region deployments.",
		},
		{
			Filter: &billanalysis.FilterV1{
				Type: billanalysis.FilterV1TypeAnd,
				Expressions: &[]billanalysis.FilterV1{
					excludeMarketplace,
					includeCompute,
				},
			},
			Dimensions:    []string{"vendor_account_name", "service", "instance_type"},
			Category:      "Compute Resources",
			Description:   "Virtual machine and compute instance costs",
			FinOpsContext: "Compute anomalies indicate infrastructure scaling or configuration changes.",
		},
	}
}
