package flexera

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

type ruleBasedDimensionBulkFake struct {
	createStatuses map[string]int
	updateCalls    []string
	rulesCalls     []string
	createErr      error
}

func (f *ruleBasedDimensionBulkFake) FinopsCustomizationsRuleBasedDimensionRuleBasedDimensionCreateWithResponse(
	_ context.Context, _ int, id string, _ FinopsCustomizationsRuleBasedDimensionRuleBasedDimensionCreateJSONRequestBody, _ ...RequestEditorFn,
) (*FinopsCustomizationsRuleBasedDimensionRuleBasedDimensionCreateResponse, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	status := http.StatusCreated
	if s, ok := f.createStatuses[id]; ok {
		status = s
	}
	return &FinopsCustomizationsRuleBasedDimensionRuleBasedDimensionCreateResponse{
		HTTPResponse: &http.Response{StatusCode: status, Status: http.StatusText(status)},
	}, nil
}

func (f *ruleBasedDimensionBulkFake) FinopsCustomizationsRuleBasedDimensionRuleBasedDimensionUpdateWithResponse(
	_ context.Context, _ int, id string, _ FinopsCustomizationsRuleBasedDimensionRuleBasedDimensionUpdateJSONRequestBody, _ ...RequestEditorFn,
) (*FinopsCustomizationsRuleBasedDimensionRuleBasedDimensionUpdateResponse, error) {
	f.updateCalls = append(f.updateCalls, id)
	return &FinopsCustomizationsRuleBasedDimensionRuleBasedDimensionUpdateResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
	}, nil
}

func (f *ruleBasedDimensionBulkFake) FinopsCustomizationsRuleBasedDimensionRulesListReplaceWithResponse(
	_ context.Context, _ int, id string, _ string, _ FinopsCustomizationsRuleBasedDimensionRulesListReplaceJSONRequestBody, _ ...RequestEditorFn,
) (*FinopsCustomizationsRuleBasedDimensionRulesListReplaceResponse, error) {
	f.rulesCalls = append(f.rulesCalls, id)
	return &FinopsCustomizationsRuleBasedDimensionRulesListReplaceResponse{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
	}, nil
}

func TestRuleBasedDimensionBulkToolValidation(t *testing.T) {
	tool := NewRuleBasedDimensionBulkTool(&ruleBasedDimensionBulkFake{})

	if _, err := tool.Invoke(context.Background(), RuleBasedDimensionBulkInput{}); err == nil {
		t.Fatalf("expected error for missing orgID")
	}
	if _, err := tool.Invoke(context.Background(), RuleBasedDimensionBulkInput{OrgID: 1}); err == nil {
		t.Fatalf("expected error for empty dimensions")
	}
	if _, err := tool.Invoke(context.Background(), RuleBasedDimensionBulkInput{
		OrgID:      1,
		Dimensions: []RuleBasedDimensionSpec{{Name: "dept"}},
	}); err == nil {
		t.Fatalf("expected error for missing id")
	}
	if _, err := tool.Invoke(context.Background(), RuleBasedDimensionBulkInput{
		OrgID:      1,
		Dimensions: []RuleBasedDimensionSpec{{ID: "rbd_department"}},
	}); err == nil {
		t.Fatalf("expected error for missing name")
	}
	if _, err := tool.Invoke(context.Background(), RuleBasedDimensionBulkInput{
		OrgID: 1,
		Dimensions: []RuleBasedDimensionSpec{{
			ID:   "rbd_department",
			Name: "Department",
			Rules: []FinopsCustomizationsRuleBasedDimensionRulePayload{
				{Value: FinopsCustomizationsRuleBasedDimensionValueExpression{}},
			},
		}},
	}); err == nil {
		t.Fatalf("expected error for rules without effectiveAt")
	}
}

func TestRuleBasedDimensionBulkToolDryRun(t *testing.T) {
	fake := &ruleBasedDimensionBulkFake{}
	tool := NewRuleBasedDimensionBulkTool(fake)

	out, err := tool.Invoke(context.Background(), RuleBasedDimensionBulkInput{
		OrgID:  123,
		DryRun: true,
		Dimensions: []RuleBasedDimensionSpec{
			{ID: "rbd_department", Name: "Department"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fake.rulesCalls) != 0 || len(fake.updateCalls) != 0 {
		t.Fatalf("dry run must not call the API")
	}
	if len(out.Results) != 1 || !out.Results[0].Skipped {
		t.Fatalf("expected one skipped result, got %+v", out.Results)
	}
}

func TestRuleBasedDimensionBulkToolCreatesAndFallsBackToUpdateOnConflict(t *testing.T) {
	fake := &ruleBasedDimensionBulkFake{
		createStatuses: map[string]int{"rbd_existing": http.StatusConflict},
	}
	tool := NewRuleBasedDimensionBulkTool(fake)

	out, err := tool.Invoke(context.Background(), RuleBasedDimensionBulkInput{
		OrgID: 123,
		Dimensions: []RuleBasedDimensionSpec{
			{ID: "rbd_new", Name: "New Dept"},
			{
				ID:          "rbd_existing",
				Name:        "Existing Dept",
				EffectiveAt: "2024-01",
				Rules: []FinopsCustomizationsRuleBasedDimensionRulePayload{
					{Value: FinopsCustomizationsRuleBasedDimensionValueExpression{}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(out.Results))
	}
	if out.Results[0].Updated {
		t.Fatalf("first dimension should have been created, not updated")
	}
	if !out.Results[1].Updated {
		t.Fatalf("second dimension should have fallen back to update")
	}
	if len(fake.updateCalls) != 1 || fake.updateCalls[0] != "rbd_existing" {
		t.Fatalf("expected update call for rbd_existing, got %+v", fake.updateCalls)
	}
	if len(fake.rulesCalls) != 1 || fake.rulesCalls[0] != "rbd_existing" {
		t.Fatalf("expected rules replace call for rbd_existing only, got %+v", fake.rulesCalls)
	}
}

func TestRuleBasedDimensionBulkToolStopsOnFirstErrorByDefault(t *testing.T) {
	fake := &ruleBasedDimensionBulkFake{createErr: errors.New("boom")}
	tool := NewRuleBasedDimensionBulkTool(fake)

	out, err := tool.Invoke(context.Background(), RuleBasedDimensionBulkInput{
		OrgID: 123,
		Dimensions: []RuleBasedDimensionSpec{
			{ID: "rbd_a", Name: "A"},
			{ID: "rbd_b", Name: "B"},
		},
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if len(out.Results) != 1 || out.Results[0].Err == nil {
		t.Fatalf("expected one failed result before stopping, got %+v", out.Results)
	}
}

func TestRuleBasedDimensionBulkToolContinuesOnErrorWhenRequested(t *testing.T) {
	fake := &ruleBasedDimensionBulkFake{createErr: errors.New("boom")}
	tool := NewRuleBasedDimensionBulkTool(fake)

	out, err := tool.Invoke(context.Background(), RuleBasedDimensionBulkInput{
		OrgID:           123,
		ContinueOnError: true,
		Dimensions: []RuleBasedDimensionSpec{
			{ID: "rbd_a", Name: "A"},
			{ID: "rbd_b", Name: "B"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Results) != 2 {
		t.Fatalf("expected both dimensions attempted, got %d", len(out.Results))
	}
	for _, r := range out.Results {
		if r.Err == nil {
			t.Fatalf("expected error on result %+v", r)
		}
	}
}
