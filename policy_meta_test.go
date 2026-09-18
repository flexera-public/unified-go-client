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
	fake := &policyMetaWorkflowFake{
		pages: []*PolicyAppliedPolicyIndexResponse{
			policyMetaPage([]PolicyFlexeraPolicyAppliedPolicy{{Id: "parent", Name: "parent"}}, "https://api.example.test/applied?skipToken=next"),
			policyMetaPage([]PolicyFlexeraPolicyAppliedPolicy{
				{Id: "child", MetaParentPolicyId: stringPointer("parent")},
				{Id: "orphan", MetaParentPolicyId: stringPointer("missing")},
				{Id: "regular"},
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
	if got := len(snapshot.MetaParents); got != 1 || snapshot.MetaParents[0].Id != "parent" {
		t.Fatalf("meta parents = %#v, want parent", snapshot.MetaParents)
	}
	if got := len(snapshot.Regular); got != 1 {
		t.Fatalf("regular policies = %d, want 1", got)
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

func TestPolicyMetaWorkflowTerminateOrphanedDeletesMissingParents(t *testing.T) {
	fake := &policyMetaWorkflowFake{
		pages: []*PolicyAppliedPolicyIndexResponse{policyMetaPage([]PolicyFlexeraPolicyAppliedPolicy{
			{Id: "parent"},
			{Id: "orphan", MetaParentPolicyId: stringPointer("missing")},
		}, "")},
	}

	out, err := NewPolicyMetaWorkflow(fake).TerminateOrphaned(context.Background(), PolicyMetaTerminationInput{
		OrgID: 1, ProjectID: 2,
	})
	if err != nil {
		t.Fatalf("TerminateOrphaned() error = %v", err)
	}
	if len(out.Deleted) != 1 || out.Deleted[0] != "orphan" {
		t.Fatalf("deleted = %#v, want orphan", out.Deleted)
	}
}

func stringPointer(value string) *string {
	return &value
}
