package flexera

import (
	"context"
	"net/http"
	"testing"
)

type policyMetaWorkflowFake struct {
	pages       []*PolicyAppliedPolicyIndexResponse
	pageCalls   int
	deleteCalls []string
	statusCalls []string
}

func (f *policyMetaWorkflowFake) PolicyAppliedPolicyIndexWithResponse(context.Context, int64, int64, *PolicyAppliedPolicyIndexParams, ...RequestEditorFn) (*PolicyAppliedPolicyIndexResponse, error) {
	page := f.pages[f.pageCalls]
	f.pageCalls++
	return page, nil
}

func (f *policyMetaWorkflowFake) PolicyAppliedPolicyDeleteWithResponse(_ context.Context, _ int64, _ int64, id string, _ ...RequestEditorFn) (*PolicyAppliedPolicyDeleteResponse, error) {
	f.deleteCalls = append(f.deleteCalls, id)
	return &PolicyAppliedPolicyDeleteResponse{HTTPResponse: &http.Response{StatusCode: http.StatusNoContent}}, nil
}

func (f *policyMetaWorkflowFake) PolicyAppliedPolicyShowStatusWithResponse(context.Context, int64, int64, string, ...RequestEditorFn) (*PolicyAppliedPolicyShowStatusResponse, error) {
	return &PolicyAppliedPolicyShowStatusResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &PolicyAppliedPolicyStatusDetail{},
	}, nil
}

func policyMetaPage(values []PolicyFlexeraPolicyAppliedPolicy, next string) *PolicyAppliedPolicyIndexResponse {
	response := &PolicyAppliedPolicyIndexResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &PolicyAppliedPolicyList{Values: &values},
	}
	if next != "" {
		response.JSON200.NextPage = &next
	}
	return response
}

func TestPolicyMetaWorkflowDiscoverResolvesRelationshipsAndPagination(t *testing.T) {
	parentRef := "ref:nam:1::policy:meta-parent-policy:other"
	fake := &policyMetaWorkflowFake{
		pages: []*PolicyAppliedPolicyIndexResponse{
			policyMetaPage([]PolicyFlexeraPolicyAppliedPolicy{{Id: "parent", Name: "parent"}}, "https://api.example.test/applied?skipToken=next"),
			policyMetaPage([]PolicyFlexeraPolicyAppliedPolicy{
				{Id: "child", MetaParentPolicyId: stringPointer("parent")},
				{Id: "orphan", MetaParentPolicyId: stringPointer("missing")},
				{Id: "ambiguous", MetaParentPolicyId: stringPointer("parent"), Parent: &PolicyParent{Ref: &parentRef}},
			}, ""),
		},
	}

	snapshot, err := NewPolicyMetaWorkflow(fake).Discover(context.Background(), 1, 2)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if got := len(snapshot.Policies); got != 4 {
		t.Fatalf("policy count = %d, want 4", got)
	}
	if got := len(snapshot.Children); got != 2 {
		t.Fatalf("child count = %d, want 2", got)
	}
	if got := len(snapshot.Orphans); got != 1 || snapshot.Orphans[0].Id != "orphan" {
		t.Fatalf("orphans = %#v, want orphan", snapshot.Orphans)
	}
	if got := len(snapshot.Ambiguous); got != 1 || snapshot.Ambiguous[0].Id != "ambiguous" {
		t.Fatalf("ambiguous = %#v, want ambiguous", snapshot.Ambiguous)
	}
}

func TestPolicyMetaWorkflowTerminateChildrenDryRunDoesNotDelete(t *testing.T) {
	fake := &policyMetaWorkflowFake{
		pages: []*PolicyAppliedPolicyIndexResponse{policyMetaPage([]PolicyFlexeraPolicyAppliedPolicy{
			{Id: "parent"},
			{Id: "child", MetaParentPolicyId: stringPointer("parent")},
		}, "")},
	}

	out, err := NewPolicyMetaWorkflow(fake).TerminateChildren(context.Background(), PolicyMetaTerminationInput{
		OrgID: 1, ProjectID: 2, ParentID: "parent", DryRun: true,
	})
	if err != nil {
		t.Fatalf("TerminateChildren() error = %v", err)
	}
	if len(out.Matched) != 1 || out.Matched[0].Id != "child" {
		t.Fatalf("matched = %#v, want child", out.Matched)
	}
	if len(fake.deleteCalls) != 0 {
		t.Fatalf("delete calls = %#v, want none", fake.deleteCalls)
	}
}

func TestPolicyMetaWorkflowTerminateOrphanedFailsClosedOnAmbiguousRelationship(t *testing.T) {
	ref := "ref:nam:1::policy:meta-parent-policy:other"
	fake := &policyMetaWorkflowFake{
		pages: []*PolicyAppliedPolicyIndexResponse{policyMetaPage([]PolicyFlexeraPolicyAppliedPolicy{
			{Id: "parent"},
			{Id: "child", MetaParentPolicyId: stringPointer("parent"), Parent: &PolicyParent{Ref: &ref}},
		}, "")},
	}

	_, err := NewPolicyMetaWorkflow(fake).TerminateOrphaned(context.Background(), PolicyMetaTerminationInput{
		OrgID: 1, ProjectID: 2,
	})
	if err == nil {
		t.Fatal("TerminateOrphaned() error = nil, want ambiguous relationship error")
	}
	if len(fake.deleteCalls) != 0 {
		t.Fatalf("delete calls = %#v, want none", fake.deleteCalls)
	}
}

func stringPointer(value string) *string {
	return &value
}
