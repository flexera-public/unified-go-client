package flexera

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// PolicyMetaWorkflowClient is the generated-client surface used by the
// applied-policy meta workflows.
type PolicyMetaWorkflowClient interface {
	PolicyAppliedPolicyIndexWithResponse(context.Context, int64, int64, *PolicyAppliedPolicyIndexParams, ...RequestEditorFn) (*PolicyAppliedPolicyIndexResponse, error)
	PolicyAppliedPolicyDeleteWithResponse(context.Context, int64, int64, string, ...RequestEditorFn) (*PolicyAppliedPolicyDeleteResponse, error)
	PolicyAppliedPolicyShowStatusWithResponse(context.Context, int64, int64, string, ...RequestEditorFn) (*PolicyAppliedPolicyShowStatusResponse, error)
}

// AppliedPolicyRelationship describes a child policy's resolved parent.
type AppliedPolicyRelationship struct {
	Child    PolicyFlexeraPolicyAppliedPolicy
	ParentID string
}

// PolicyMetaSnapshot is the relationship-aware view used by meta-policy
// commands. Orphans and ambiguous relationships are never deletion targets
// unless a caller resolves them explicitly.
type PolicyMetaSnapshot struct {
	Policies  []PolicyFlexeraPolicyAppliedPolicy
	Children  []AppliedPolicyRelationship
	Orphans   []PolicyFlexeraPolicyAppliedPolicy
	Ambiguous []PolicyFlexeraPolicyAppliedPolicy
}

// PolicyMetaTerminationInput controls a destructive meta-policy workflow.
type PolicyMetaTerminationInput struct {
	OrgID     int64
	ProjectID int64
	ParentID  string
	DryRun    bool
}

// PolicyMetaTerminationOutput summarizes a termination workflow.
type PolicyMetaTerminationOutput struct {
	Matched []PolicyFlexeraPolicyAppliedPolicy
	Deleted []string
}

// PolicyMetaStatus is the status of one applied policy in an audit.
type PolicyMetaStatus struct {
	Policy  PolicyFlexeraPolicyAppliedPolicy
	Details PolicyAppliedPolicyStatusDetail
}

// PolicyMetaAuditOutput contains status details for a policy group.
type PolicyMetaAuditOutput struct {
	Statuses []PolicyMetaStatus
}

// PolicyMetaWorkflow provides curated, relationship-aware applied-policy
// operations for next-generation CLI commands.
type PolicyMetaWorkflow struct {
	client PolicyMetaWorkflowClient
}

// NewPolicyMetaWorkflow constructs a policy meta workflow.
func NewPolicyMetaWorkflow(client PolicyMetaWorkflowClient) *PolicyMetaWorkflow {
	return &PolicyMetaWorkflow{client: client}
}

// Discover lists all applied policies in a project and resolves their
// parent/child relationships from both the current parent reference and the
// deprecated metaParentPolicyId field.
func (w *PolicyMetaWorkflow) Discover(ctx context.Context, orgID, projectID int64) (PolicyMetaSnapshot, error) {
	if w == nil || w.client == nil {
		return PolicyMetaSnapshot{}, fmt.Errorf("policy meta workflow: client is required")
	}
	if orgID <= 0 || projectID <= 0 {
		return PolicyMetaSnapshot{}, fmt.Errorf("policy meta workflow: orgID and projectID must be positive")
	}

	policies, err := w.listAppliedPolicies(ctx, orgID, projectID)
	if err != nil {
		return PolicyMetaSnapshot{}, err
	}

	known := make(map[string]struct{}, len(policies))
	for _, policy := range policies {
		known[policy.Id] = struct{}{}
	}

	snapshot := PolicyMetaSnapshot{Policies: policies}
	for _, policy := range policies {
		parentID, ambiguous := appliedPolicyParentID(policy)
		if ambiguous {
			snapshot.Ambiguous = append(snapshot.Ambiguous, policy)
			continue
		}
		if parentID == "" {
			continue
		}
		snapshot.Children = append(snapshot.Children, AppliedPolicyRelationship{
			Child:    policy,
			ParentID: parentID,
		})
		if _, exists := known[parentID]; !exists {
			snapshot.Orphans = append(snapshot.Orphans, policy)
		}
	}
	return snapshot, nil
}

// TerminateChildren deletes all unambiguous children of ParentID. In DryRun
// mode it only discovers and returns the matched policies.
func (w *PolicyMetaWorkflow) TerminateChildren(ctx context.Context, in PolicyMetaTerminationInput) (PolicyMetaTerminationOutput, error) {
	parentID := strings.TrimSpace(in.ParentID)
	if parentID == "" {
		return PolicyMetaTerminationOutput{}, fmt.Errorf("meta-terminate-children: parentID is required")
	}
	snapshot, err := w.Discover(ctx, in.OrgID, in.ProjectID)
	if err != nil {
		return PolicyMetaTerminationOutput{}, err
	}
	if len(snapshot.Ambiguous) > 0 {
		return PolicyMetaTerminationOutput{}, fmt.Errorf("meta-terminate-children: refusing to act with %d ambiguous relationship(s)", len(snapshot.Ambiguous))
	}

	out := PolicyMetaTerminationOutput{}
	for _, relationship := range snapshot.Children {
		if relationship.ParentID == parentID {
			out.Matched = append(out.Matched, relationship.Child)
		}
	}
	if in.DryRun {
		return out, nil
	}
	for _, child := range out.Matched {
		response, err := w.client.PolicyAppliedPolicyDeleteWithResponse(ctx, in.OrgID, in.ProjectID, child.Id)
		if err != nil {
			return out, fmt.Errorf("meta-terminate-children: delete %q: %w", child.Id, err)
		}
		if response == nil {
			return out, fmt.Errorf("meta-terminate-children: delete applied policy returned no response")
		}
		if err := policyMetaResponseError(response, "delete applied policy"); err != nil {
			return out, fmt.Errorf("meta-terminate-children: %w", err)
		}
		out.Deleted = append(out.Deleted, child.Id)
	}
	return out, nil
}

// TerminateOrphaned deletes children whose parent reference is present but
// whose parent is absent from the project. Ambiguous relationships always
// fail closed. In DryRun mode it only returns the candidates.
func (w *PolicyMetaWorkflow) TerminateOrphaned(ctx context.Context, in PolicyMetaTerminationInput) (PolicyMetaTerminationOutput, error) {
	snapshot, err := w.Discover(ctx, in.OrgID, in.ProjectID)
	if err != nil {
		return PolicyMetaTerminationOutput{}, err
	}
	if len(snapshot.Ambiguous) > 0 {
		return PolicyMetaTerminationOutput{}, fmt.Errorf("meta-terminate-orphaned: refusing to act with %d ambiguous relationship(s)", len(snapshot.Ambiguous))
	}

	out := PolicyMetaTerminationOutput{Matched: append([]PolicyFlexeraPolicyAppliedPolicy(nil), snapshot.Orphans...)}
	if in.DryRun {
		return out, nil
	}
	for _, orphan := range out.Matched {
		response, err := w.client.PolicyAppliedPolicyDeleteWithResponse(ctx, in.OrgID, in.ProjectID, orphan.Id)
		if err != nil {
			return out, fmt.Errorf("meta-terminate-orphaned: delete %q: %w", orphan.Id, err)
		}
		if response == nil {
			return out, fmt.Errorf("meta-terminate-orphaned: delete applied policy returned no response")
		}
		if err := policyMetaResponseError(response, "delete applied policy"); err != nil {
			return out, fmt.Errorf("meta-terminate-orphaned: %w", err)
		}
		out.Deleted = append(out.Deleted, orphan.Id)
	}
	return out, nil
}

// Audit returns status details for every applied policy in a project.
func (w *PolicyMetaWorkflow) Audit(ctx context.Context, orgID, projectID int64) (PolicyMetaAuditOutput, error) {
	snapshot, err := w.Discover(ctx, orgID, projectID)
	if err != nil {
		return PolicyMetaAuditOutput{}, err
	}
	out := PolicyMetaAuditOutput{Statuses: make([]PolicyMetaStatus, 0, len(snapshot.Policies))}
	for _, policy := range snapshot.Policies {
		response, err := w.client.PolicyAppliedPolicyShowStatusWithResponse(ctx, orgID, projectID, policy.Id)
		if err != nil {
			return out, fmt.Errorf("audit applied policy %q: %w", policy.Id, err)
		}
		if response == nil {
			return out, fmt.Errorf("audit applied policy %q: show applied policy status returned no response", policy.Id)
		}
		if err := policyMetaResponseError(response, "show applied policy status"); err != nil {
			return out, fmt.Errorf("audit applied policy %q: %w", policy.Id, err)
		}
		if response == nil || response.JSON200 == nil {
			return out, fmt.Errorf("audit applied policy %q: expected a successful status response", policy.Id)
		}
		out.Statuses = append(out.Statuses, PolicyMetaStatus{Policy: policy, Details: *response.JSON200})
	}
	return out, nil
}

func (w *PolicyMetaWorkflow) listAppliedPolicies(ctx context.Context, orgID, projectID int64) ([]PolicyFlexeraPolicyAppliedPolicy, error) {
	var all []PolicyFlexeraPolicyAppliedPolicy
	var skipToken *string
	seenTokens := make(map[string]struct{})
	for pages := 0; ; pages++ {
		if pages >= maxPaginationPages {
			return nil, fmt.Errorf("list applied policies: exceeded max page limit (%d)", maxPaginationPages)
		}
		params := &PolicyAppliedPolicyIndexParams{SkipToken: skipToken}
		response, err := w.client.PolicyAppliedPolicyIndexWithResponse(ctx, orgID, projectID, params)
		if err != nil {
			return nil, fmt.Errorf("list applied policies: %w", err)
		}
		if response == nil {
			return nil, fmt.Errorf("list applied policies: expected a successful response")
		}
		if err := policyMetaResponseError(response, "list applied policies"); err != nil {
			return nil, err
		}
		if response == nil || response.JSON200 == nil {
			return nil, fmt.Errorf("list applied policies: expected a successful response")
		}
		if response.JSON200.Values != nil {
			all = append(all, (*response.JSON200.Values)...)
		}
		if response.JSON200.NextPage == nil || strings.TrimSpace(*response.JSON200.NextPage) == "" {
			return all, nil
		}
		token, err := skipTokenFromNextPage(*response.JSON200.NextPage)
		if err != nil {
			return nil, fmt.Errorf("list applied policies: %w", err)
		}
		if _, seen := seenTokens[token]; seen {
			return nil, fmt.Errorf("list applied policies: repeated skipToken %q", token)
		}
		seenTokens[token] = struct{}{}
		skipToken = &token
	}
}

func appliedPolicyParentID(policy PolicyFlexeraPolicyAppliedPolicy) (string, bool) {
	legacyID := strings.TrimSpace(valueOrEmpty(policy.MetaParentPolicyId))
	refID, refOK := policyMetaParentID(policy.Parent)
	if legacyID != "" && refOK && legacyID != refID {
		return "", true
	}
	if refOK {
		return refID, false
	}
	if policy.Parent != nil && policy.Parent.Ref != nil && strings.TrimSpace(*policy.Parent.Ref) != "" {
		return "", true
	}
	return legacyID, false
}

func policyMetaParentID(parent *PolicyParent) (string, bool) {
	if parent == nil || parent.Ref == nil {
		return "", false
	}
	parts := strings.Split(strings.TrimSpace(*parent.Ref), ":")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "meta-parent-policy" && parts[i+1] != "" {
			return parts[i+1], true
		}
	}
	return "", false
}

func skipTokenFromNextPage(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid nextPage URL %q: %w", raw, err)
	}
	token := parsed.Query().Get("skipToken")
	if token == "" {
		return "", fmt.Errorf("nextPage URL %q has no skipToken", raw)
	}
	return token, nil
}

func policyMetaResponseError(response interface{ StatusCode() int }, action string) error {
	if response == nil {
		return fmt.Errorf("%s returned no response", action)
	}
	code := response.StatusCode()
	if code < http.StatusOK || code >= http.StatusMultipleChoices {
		return fmt.Errorf("%s returned HTTP %d", action, code)
	}
	return nil
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
