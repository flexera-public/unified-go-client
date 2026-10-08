package flexera

// Service identifies the Flexera One API service that owns an operation, as
// recorded by the unified spec's x-flexera-service extension. Constants and
// lookup data are generated into client_gen_services.go.
type Service string

// ServiceInfo describes one API service from the unified spec's
// x-flexera-services registry.
type ServiceInfo struct {
	ID                Service
	Title             string   // display name, e.g. "Bill Analysis"
	Name              string   // upstream spec name, e.g. "RightScale Bill Analysis API"
	Vendor            string   // flexera | rightscale
	Version           string   // upstream API version; empty when unversioned
	SpecID            string   // unified-openapi specs.yaml id
	OperationIDPrefix string   // prefix on generated operationIds and types, e.g. "BillAnalysis"
	SourceURL         string   // upstream OpenAPI document URL
	Tags              []string // OpenAPI tags with operations from this service
}

// Services returns every service in the unified spec, sorted by ID.
func Services() []ServiceInfo {
	out := make([]ServiceInfo, len(serviceRegistry))
	for i, svc := range serviceRegistry {
		svc.Tags = append([]string(nil), svc.Tags...)
		out[i] = svc
	}
	return out
}

// LookupService returns the registry entry for id.
func LookupService(id Service) (ServiceInfo, bool) {
	for _, svc := range serviceRegistry {
		if svc.ID == id {
			svc.Tags = append([]string(nil), svc.Tags...)
			return svc, true
		}
	}
	return ServiceInfo{}, false
}

// ServiceOf returns the service that owns operationID (e.g.
// "BillAnalysis_costs_aggregated" -> ServiceBillAnalysis).
func ServiceOf(operationID string) (Service, bool) {
	svc, ok := operationServices[operationID]
	return svc, ok
}
