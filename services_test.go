package flexera

import "testing"

func TestServiceOf(t *testing.T) {
	cases := map[string]Service{
		"BillAnalysis_costs_aggregated": ServiceBillAnalysis,
		"Iam_Project_Index":             ServiceIam,
		"Grs_Project_indexForOrg":       ServiceGrs,
		"Auth_Token_token":              ServiceAuth,
	}
	for operationID, want := range cases {
		if got, ok := ServiceOf(operationID); !ok || got != want {
			t.Errorf("ServiceOf(%q) = %q, %v; want %q", operationID, got, ok, want)
		}
	}
	if _, ok := ServiceOf("Nope_nope"); ok {
		t.Error("ServiceOf(unknown) reported ok")
	}
}

func TestServicesRegistry(t *testing.T) {
	services := Services()
	if len(services) == 0 {
		t.Fatal("no services generated")
	}
	seen := map[Service]bool{}
	for _, svc := range services {
		if svc.Title == "" || svc.OperationIDPrefix == "" || len(svc.Tags) == 0 {
			t.Errorf("incomplete service entry %#v", svc)
		}
		seen[svc.ID] = true
	}
	for operationID, svc := range operationServices {
		if !seen[svc] {
			t.Errorf("operation %s maps to unregistered service %q", operationID, svc)
		}
	}

	info, ok := LookupService(ServiceBillAnalysis)
	if !ok || info.Title != "Bill Analysis" {
		t.Fatalf("LookupService(bill_analysis) = %#v, %v", info, ok)
	}
	info.Tags[0] = "mutated"
	if again, _ := LookupService(ServiceBillAnalysis); again.Tags[0] == "mutated" {
		t.Error("LookupService returned shared Tags slice")
	}
}
