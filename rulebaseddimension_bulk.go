package flexera

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// RuleBasedDimensionBulkClient is the generated-client surface used by the
// rule-based-dimension bulk workflow.
type RuleBasedDimensionBulkClient interface {
	FinopsCustomizationsRuleBasedDimensionRuleBasedDimensionCreateWithResponse(
		ctx context.Context,
		orgId int,
		id string,
		body FinopsCustomizationsRuleBasedDimensionRuleBasedDimensionCreateJSONRequestBody,
		reqEditors ...RequestEditorFn,
	) (*FinopsCustomizationsRuleBasedDimensionRuleBasedDimensionCreateResponse, error)
	FinopsCustomizationsRuleBasedDimensionRuleBasedDimensionUpdateWithResponse(
		ctx context.Context,
		orgId int,
		id string,
		body FinopsCustomizationsRuleBasedDimensionRuleBasedDimensionUpdateJSONRequestBody,
		reqEditors ...RequestEditorFn,
	) (*FinopsCustomizationsRuleBasedDimensionRuleBasedDimensionUpdateResponse, error)
	FinopsCustomizationsRuleBasedDimensionRulesListReplaceWithResponse(
		ctx context.Context,
		orgId int,
		id string,
		effectiveAt string,
		body FinopsCustomizationsRuleBasedDimensionRulesListReplaceJSONRequestBody,
		reqEditors ...RequestEditorFn,
	) (*FinopsCustomizationsRuleBasedDimensionRulesListReplaceResponse, error)
}

// RuleBasedDimensionSpec describes one rule-based dimension to create or
// update, with an optional rules list to apply for a given effective month.
// Rules and EffectiveAt may be left empty to only create or rename the
// dimension without touching its rules.
type RuleBasedDimensionSpec struct {
	ID          string
	Name        string
	EffectiveAt string
	Rules       []FinopsCustomizationsRuleBasedDimensionRulePayload
}

// RuleBasedDimensionBulkInput is the input to the bulk create/update
// workflow. Dimensions are processed in order.
type RuleBasedDimensionBulkInput struct {
	OrgID int
	// Dimensions is the batch of rule-based dimensions to create or update,
	// typically produced from a CSV file by the caller.
	Dimensions []RuleBasedDimensionSpec
	// DryRun reports the dimensions that would be processed without making
	// any API calls.
	DryRun bool
	// ContinueOnError processes the remaining dimensions after a failure
	// instead of stopping at the first one.
	ContinueOnError bool
}

// RuleBasedDimensionBulkResult is the per-dimension outcome of a bulk run.
type RuleBasedDimensionBulkResult struct {
	ID string
	// Updated is true when the dimension already existed and was updated
	// instead of created.
	Updated bool
	// Skipped is true in DryRun mode.
	Skipped bool
	Err     error
}

// RuleBasedDimensionBulkOutput is the result of a bulk create/update run.
type RuleBasedDimensionBulkOutput struct {
	Results []RuleBasedDimensionBulkResult
}

// RuleBasedDimensionBulkTool creates or updates many rule-based dimensions
// (and their rules for a given effective month) from a batch of specs, e.g.
// generated from a CSV file by the caller. It orchestrates the
// create-dimension, fallback-to-update, and replace-rules calls that
// together make up a single dimension "upsert".
type RuleBasedDimensionBulkTool struct {
	client RuleBasedDimensionBulkClient
}

// NewRuleBasedDimensionBulkTool constructs the rule-based-dimension bulk
// workflow.
func NewRuleBasedDimensionBulkTool(client RuleBasedDimensionBulkClient) *RuleBasedDimensionBulkTool {
	return &RuleBasedDimensionBulkTool{client: client}
}

func (*RuleBasedDimensionBulkTool) Name() string {
	return "rule-based-dimension bulk"
}

func (*RuleBasedDimensionBulkTool) Description() string {
	return "Create or update many rule-based dimensions and their rules from a batch of specs"
}

// Invoke validates the entire batch before making any API calls. Once
// validated, it processes dimensions in order, falling back to an update
// when a dimension already exists.
func (t *RuleBasedDimensionBulkTool) Invoke(ctx context.Context, in RuleBasedDimensionBulkInput) (RuleBasedDimensionBulkOutput, error) {
	if t == nil || t.client == nil {
		return RuleBasedDimensionBulkOutput{}, fmt.Errorf("rule-based-dimension bulk: client is required")
	}
	if in.OrgID <= 0 {
		return RuleBasedDimensionBulkOutput{}, fmt.Errorf("rule-based-dimension bulk: orgID must be positive")
	}
	if len(in.Dimensions) == 0 {
		return RuleBasedDimensionBulkOutput{}, fmt.Errorf("rule-based-dimension bulk: at least one dimension is required")
	}
	for i, dim := range in.Dimensions {
		id := strings.TrimSpace(dim.ID)
		name := strings.TrimSpace(dim.Name)
		if id == "" {
			return RuleBasedDimensionBulkOutput{}, fmt.Errorf("rule-based-dimension bulk: dimension %d: id is required", i)
		}
		if name == "" {
			return RuleBasedDimensionBulkOutput{}, fmt.Errorf("rule-based-dimension bulk: dimension %d (%s): name is required", i, id)
		}
		if len(dim.Rules) > 0 && strings.TrimSpace(dim.EffectiveAt) == "" {
			return RuleBasedDimensionBulkOutput{}, fmt.Errorf("rule-based-dimension bulk: dimension %d (%s): effectiveAt is required when rules are provided", i, id)
		}
	}

	out := RuleBasedDimensionBulkOutput{Results: make([]RuleBasedDimensionBulkResult, 0, len(in.Dimensions))}
	if in.DryRun {
		for _, dim := range in.Dimensions {
			out.Results = append(out.Results, RuleBasedDimensionBulkResult{ID: dim.ID, Skipped: true})
		}
		return out, nil
	}

	for _, dim := range in.Dimensions {
		result := RuleBasedDimensionBulkResult{ID: dim.ID}
		if err := t.upsertDimension(ctx, in.OrgID, dim, &result); err != nil {
			result.Err = err
			out.Results = append(out.Results, result)
			if !in.ContinueOnError {
				return out, err
			}
			continue
		}
		out.Results = append(out.Results, result)
	}
	return out, nil
}

func (t *RuleBasedDimensionBulkTool) upsertDimension(ctx context.Context, orgID int, dim RuleBasedDimensionSpec, result *RuleBasedDimensionBulkResult) error {
	createResp, err := t.client.FinopsCustomizationsRuleBasedDimensionRuleBasedDimensionCreateWithResponse(
		ctx, orgID, dim.ID, FinopsCustomizationsRuleBasedDimensionCreateRequestBody{Name: dim.Name},
	)
	if err != nil {
		return fmt.Errorf("create dimension %q: %w", dim.ID, err)
	}
	if createResp == nil {
		return fmt.Errorf("create dimension %q: no response", dim.ID)
	}
	switch {
	case createResp.StatusCode() >= 200 && createResp.StatusCode() < 300:
		// created
	case createResp.StatusCode() == http.StatusConflict:
		result.Updated = true
		updateResp, err := t.client.FinopsCustomizationsRuleBasedDimensionRuleBasedDimensionUpdateWithResponse(
			ctx, orgID, dim.ID, FinopsCustomizationsRuleBasedDimensionCreateRequestBody{Name: dim.Name},
		)
		if err != nil {
			return fmt.Errorf("update dimension %q: %w", dim.ID, err)
		}
		if updateResp == nil {
			return fmt.Errorf("update dimension %q: no response", dim.ID)
		}
		if updateResp.StatusCode() < 200 || updateResp.StatusCode() >= 300 {
			return fmt.Errorf("update dimension %q: unexpected status %s: %s", dim.ID, updateResp.Status(), string(updateResp.Body))
		}
	default:
		return fmt.Errorf("create dimension %q: unexpected status %s: %s", dim.ID, createResp.Status(), string(createResp.Body))
	}

	if len(dim.Rules) == 0 {
		return nil
	}
	rulesResp, err := t.client.FinopsCustomizationsRuleBasedDimensionRulesListReplaceWithResponse(
		ctx, orgID, dim.ID, dim.EffectiveAt, FinopsCustomizationsRulesListReplaceRequestBody{Rules: dim.Rules},
	)
	if err != nil {
		return fmt.Errorf("replace rules for dimension %q: %w", dim.ID, err)
	}
	if rulesResp == nil {
		return fmt.Errorf("replace rules for dimension %q: no response", dim.ID)
	}
	if rulesResp.StatusCode() < 200 || rulesResp.StatusCode() >= 300 {
		return fmt.Errorf("replace rules for dimension %q: unexpected status %s: %s", dim.ID, rulesResp.Status(), string(rulesResp.Body))
	}
	return nil
}
