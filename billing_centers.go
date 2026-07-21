package flexera

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	bcs "github.com/flexera-public/unified-go-client/rightscale/billing_center_service"
)

// BillingCenterClient is the subset of the generated billing_center_service
// client used by the resolver. It is defined as an interface so tests can
// stub the index call.
type BillingCenterClient interface {
	BillingCentersIndexWithResponse(
		ctx context.Context,
		orgID int,
		params *bcs.BillingCentersIndexParams,
		reqEditors ...bcs.RequestEditorFn,
	) (*bcs.BillingCentersIndexResponse, error)
}

// billingCenter is the minimal shape returned by /billing-centers index that
// the resolver cares about. The generated client returns raw bytes (the
// upstream Swagger 2.0 spec did not model the response), so we unmarshal
// against this local type.
type billingCenter struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	ParentID *string `json:"parent_id,omitempty"`
}

// BillingCenterResolver caches the top-level billing-center IDs for each
// org so repeated cost requests during a single process don't refetch.
type BillingCenterResolver struct {
	client BillingCenterClient

	mu    sync.Mutex
	cache map[int][]string // orgID → top-level BC IDs
}

// NewBillingCenterResolver constructs a resolver backed by the given client.
func NewBillingCenterResolver(client BillingCenterClient) *BillingCenterResolver {
	return &BillingCenterResolver{
		client: client,
		cache:  make(map[int][]string),
	}
}

// TopLevelIDs returns every top-level billing-center ID (parent_id empty) for
// the given org. Result is cached per resolver instance.
func (r *BillingCenterResolver) TopLevelIDs(ctx context.Context, orgID int) ([]string, error) {
	r.mu.Lock()
	if ids, ok := r.cache[orgID]; ok {
		r.mu.Unlock()
		return ids, nil
	}
	r.mu.Unlock()

	resp, err := r.client.BillingCentersIndexWithResponse(ctx, orgID, nil)
	if err != nil {
		return nil, fmt.Errorf("billing-centers index: %w", err)
	}
	if resp.HTTPResponse == nil || resp.HTTPResponse.StatusCode != http.StatusOK {
		status := 0
		if resp.HTTPResponse != nil {
			status = resp.HTTPResponse.StatusCode
		}
		return nil, fmt.Errorf("billing-centers index returned status %d: %s", status, string(resp.Body))
	}

	var bcs []billingCenter
	if err := json.Unmarshal(resp.Body, &bcs); err != nil {
		return nil, fmt.Errorf("decode billing-centers: %w", err)
	}

	ids := make([]string, 0, len(bcs))
	for _, bc := range bcs {
		if bc.ParentID == nil || *bc.ParentID == "" {
			ids = append(ids, bc.ID)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no top-level billing centers found for org %d", orgID)
	}

	r.mu.Lock()
	r.cache[orgID] = ids
	r.mu.Unlock()
	return ids, nil
}

// ResolveOrUse returns ids unchanged when non-empty; otherwise calls
// TopLevelIDs to fetch the default set. Useful for command helpers that
// accept an optional --billing-center-ids flag.
func (r *BillingCenterResolver) ResolveOrUse(ctx context.Context, orgID int, ids []string) ([]string, error) {
	if len(ids) > 0 {
		return ids, nil
	}
	return r.TopLevelIDs(ctx, orgID)
}
