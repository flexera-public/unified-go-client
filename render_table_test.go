package flexera

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestWriteTable_CollectedMapsMatchTypedResponses(t *testing.T) {
	tests := []struct {
		kind  string
		typed any
	}{
		{string(FinopsBudgetList), &BudgetBudgetList{}},
		{string(FinopsCloudVendorAccountList), &BudgetCloudVendorAccountsList{}},
		{string(FinopsGenericBillList), &FinopsOnboardingGenericBillConnectItemList{}},
		{string(FinopsRuleBasedDimensionList), &FinopsCustomizationsRuleBasedDimensionList{}},
		{string(FinopsTagDimensionList), &FinopsCustomizationsTagDimensionList{}},
		{string(IamOrgUserList), &IamUserList{}},
		{string(IamUserOrgListKindIamUserOrgList), &IamUserOrgList{}},
		{string(IamOrgGroupList), &IamGroupList{}},
		{string(IamAccessRuleListKindIamAccessRuleList), &IamAccessRuleList{}},
		{string(IamApiEventCollection), &IamAPIEventCollection{}},
		{string(IamEventCollectionKindIamEventCollection), &IamEventCollection{}},
		{"iam:event-collection", &IamEventCollection{}},
		{string(IamCustomizationTypeListKindIamCustomizationTypeList), &IamCustomizationTypeList{}},
		{"iam:customization-type-list", &IamCustomizationTypeList{}},
		{string(IamCustomizationListKindIamCustomizationList), &IamCustomizationList{}},
		{"iam:customization-list", &IamCustomizationList{}},
		{string(IamIpAccessControlRuleList), &IamIPAccessControlRuleList{}},
		{"policy:published-template-list", &PolicyPublishedTemplateList{}},
		{"policy:custom-catalog-template-list", &PolicyCustomCatalogTemplateList{}},
		{string(PolicyCustomizationTypeList), &PolicyCustomizationTypesList{}},
		{string(PolicyCustomizationValueList), &PolicyCustomizationValuesList{}},
		{string(PolicyPolicyManagerListKindPolicyPolicyManagerList), &PolicyPolicyManagerList{}},
		{string(PolicyIncidentAggregateListKindPolicyIncidentAggregateList), &PolicyIncidentAggregateList{}},
		{string(PolicyPolicyAggregateListKindPolicyPolicyAggregateList), &PolicyPolicyAggregateList{}},
		{string(PolicyAppliedPolicyListKindPolicyAppliedPolicyList), &PolicyAppliedPolicyList{}},
		{string(PolicyActionStatusListKindPolicyActionStatusList), &PolicyActionStatusList{}},
		{string(PolicyArchivedIncidentListKindPolicyArchivedIncidentList), &PolicyArchivedIncidentList{}},
		{string(PolicyPolicyTemplateListKindPolicyPolicyTemplateList), &PolicyPolicyTemplateList{}},
		{string(PolicyUnmanagedIncidentList), &PolicyIncidentAggregateUnmanaged{}},
		{string(PolicyUnmanagedAppliedPolicyList), &PolicyPolicyAggregateUnmanaged{}},
	}
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			for _, values := range []string{"[]", "[{}]"} {
				t.Run(values, func(t *testing.T) {
					if err := json.Unmarshal([]byte(fmt.Sprintf(`{"kind":%q,"values":%s}`, tt.kind, values)), tt.typed); err != nil {
						t.Fatal(err)
					}
					got, err := CollectPages(context.Background(), false, nil, func(context.Context, *string) (any, error) {
						return tt.typed, nil
					})
					if err != nil {
						t.Fatal(err)
					}
					assertSameTable(t, got, tt.typed)
				})
			}
		})
	}
}

func TestWriteTable_CollectedUsersIncludesAllRows(t *testing.T) {
	calls := 0
	got, err := CollectPages(context.Background(), false, nil, func(context.Context, *string) (any, error) {
		calls++
		page := &IamUserList{Kind: IamOrgUserList, Values: []IamFlexeraIamUser{{Id: 9007199254740993, Email: "first@example.test"}}}
		if calls == 1 {
			page.NextPage = stringPtr("https://example.test?skipToken=2")
		} else if calls == 2 {
			page.Values = []IamFlexeraIamUser{{Id: 2, Email: "second@example.test"}}
		} else {
			t.Fatal("unexpected fetch")
		}
		return page, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := WriteTable(&output, got); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"9007199254740993", "first@example.test", "second@example.test"} {
		if !strings.Contains(output.String(), text) {
			t.Fatalf("table missing %q:\n%s", text, &output)
		}
	}
	if calls != 2 || strings.Count(output.String(), "\n") != 3 {
		t.Fatalf("expected header and two rows after two fetches: calls=%d, table=%q", calls, &output)
	}
}

func TestWriteTable_MapErrors(t *testing.T) {
	for _, kind := range []string{"", "unknown", "cred:credentials-list", "policy#published-template-list"} {
		t.Run(kind, func(t *testing.T) {
			var output bytes.Buffer
			err := WriteTable(&output, map[string]any{"kind": kind, "values": []any{}})
			if err == nil || !strings.Contains(err.Error(), "CollectPagesAs") {
				t.Fatalf("expected actionable unsupported/ambiguous type error, got %v", err)
			}
			if output.Len() != 0 {
				t.Fatalf("wrote output for ambiguous/unsupported type: %s", &output)
			}
		})
	}
	for _, values := range []any{make(chan int), "not an array"} {
		if err := WriteTable(&bytes.Buffer{}, map[string]any{"kind": string(IamOrgUserList), "values": values}); err == nil {
			t.Fatalf("expected error for invalid values: %v", values)
		}
	}
}

func TestCollectPagesAs_TableOutput(t *testing.T) {
	t.Run("users without kind, two pages", func(t *testing.T) {
		checkTypedTableCollection[*IamUserList](t,
			[]string{
				`{"nextPage":"https://example.test?skipToken=2","values":[{"id":9007199254740993,"email":"first@example.test"}]}`,
				`{"values":[{"id":2,"email":"second@example.test"}]}`,
			},
			`{"values":[{"id":9007199254740993,"email":"first@example.test"},{"id":2,"email":"second@example.test"}]}`)
	})
	t.Run("organization credentials", func(t *testing.T) {
		checkTypedTableCollection[*CredOrganizationCredentialsList](t,
			[]string{`{"kind":"cred:credentials-list","values":[{"id":"org","orgId":123}]}`},
			`{"kind":"cred:credentials-list","values":[{"id":"org","orgId":123}]}`)
	})
	t.Run("project credentials", func(t *testing.T) {
		checkTypedTableCollection[*CredProjectCredentialsList](t,
			[]string{`{"kind":"cred:credentials-list","values":[{"id":"project","projectId":456}]}`},
			`{"kind":"cred:credentials-list","values":[{"id":"project","projectId":456}]}`)
	})
	t.Run("empty project credentials", func(t *testing.T) {
		checkTypedTableCollection[*CredProjectCredentialsList](t,
			[]string{`{"kind":"cred:credentials-list","values":[]}`},
			`{"kind":"cred:credentials-list","values":[]}`)
	})
	t.Run("published templates with shared kind", func(t *testing.T) {
		checkTypedTableCollection[*PolicyPublishedTemplateList](t,
			[]string{`{"kind":"policy#published-template-list","values":[{"id":"published","name":"Published"}]}`},
			`{"kind":"policy#published-template-list","values":[{"id":"published","name":"Published"}]}`)
	})
	t.Run("custom catalog templates with shared kind", func(t *testing.T) {
		checkTypedTableCollection[*PolicyCustomCatalogTemplateList](t,
			[]string{`{"kind":"policy#published-template-list","values":[{"id":"catalog","name":"Catalog"}]}`},
			`{"kind":"policy#published-template-list","values":[{"id":"catalog","name":"Catalog"}]}`)
	})
	t.Run("SCIM groups without kind", func(t *testing.T) {
		checkTypedTableCollection[*IamSCIMGroupList](t,
			[]string{`{"Resources":[{"id":"group","displayName":"Group"}],"totalResults":1}`},
			`{"Resources":[{"id":"group","displayName":"Group"}],"totalResults":1}`)
	})
	t.Run("empty SCIM users without kind", func(t *testing.T) {
		checkTypedTableCollection[*IamSCIMUserList](t, []string{`{"Resources":[]}`}, `{"Resources":[]}`)
	})
	t.Run("bare role array", func(t *testing.T) {
		checkTypedTableCollection[*IamRoleCollection](t, []string{`[{"id":1,"name":"Role"}]`}, `[{"id":1,"name":"Role"}]`)
	})
}

func checkTypedTableCollection[T any](t *testing.T, pages []string, wantJSON string) {
	t.Helper()
	for _, noPaginate := range []bool{false, true} {
		t.Run(fmt.Sprintf("noPaginate=%t", noPaginate), func(t *testing.T) {
			calls := 0
			got, err := CollectPagesAs(context.Background(), noPaginate, nil, func(_ context.Context, token *string) (T, error) {
				if calls >= len(pages) {
					t.Fatal("unexpected fetch")
				}
				if calls > 0 && (token == nil || *token != "2") {
					t.Fatalf("unexpected token: %v", token)
				}
				var page T
				if err := json.Unmarshal([]byte(pages[calls]), &page); err != nil {
					t.Fatal(err)
				}
				calls++
				return page, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			wantCalls := len(pages)
			rawWant := wantJSON
			if noPaginate {
				wantCalls = 1
				rawWant = pages[0]
			}
			if calls != wantCalls {
				t.Fatalf("fetch calls = %d, want %d", calls, wantCalls)
			}
			var want T
			if err := json.Unmarshal([]byte(rawWant), &want); err != nil {
				t.Fatal(err)
			}
			assertSameTable(t, got, want)
		})
	}
}

func assertSameTable(t *testing.T, got, want any) {
	t.Helper()
	var gotTable, wantTable bytes.Buffer
	if err := WriteTable(&gotTable, got); err != nil {
		t.Fatal(err)
	}
	if err := WriteTable(&wantTable, want); err != nil {
		t.Fatal(err)
	}
	if gotTable.String() != wantTable.String() {
		t.Fatalf("collected table differs from typed table:\ngot:\n%swant:\n%s", &gotTable, &wantTable)
	}
}
