package flexera

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// GenerateQueryRequestBody describes a natural-language GraphQL query request.
type GenerateQueryRequestBody struct {
	Prompt       string `json:"prompt,omitempty"`
	Query        string `json:"query,omitempty"`
	ModifyPrompt string `json:"modifyPrompt,omitempty"`
	Indent       bool   `json:"indent,omitempty"`
}

// GenerateQueryResponse contains a generated GraphQL query.
type GenerateQueryResponse struct {
	Prompt string `json:"prompt"`
	Query  string `json:"query"`
	Valid  bool   `json:"valid"`
}

// GenerateQuery calls the GraphQL query-generation endpoint, which is not yet
// described by unified-openapi.
func (c *Client) GenerateQuery(ctx context.Context, orgID int64, body GenerateQueryRequestBody, reqEditors ...RequestEditorFn) (*GenerateQueryResponse, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal generate query request: %w", err)
	}
	base, err := url.Parse(c.Server)
	if err != nil {
		return nil, fmt.Errorf("parse server URL: %w", err)
	}
	endpoint := base.JoinPath("graphql", "v1", "orgs", fmt.Sprint(orgID), "generate")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create generate query request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if err := c.applyEditors(ctx, req, reqEditors); err != nil {
		return nil, fmt.Errorf("apply generate query request editors: %w", err)
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute generate query request: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read generate query response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("generate query returned status %d: %s", resp.StatusCode, string(data))
	}
	var result GenerateQueryResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode generate query response: %w", err)
	}
	return &result, nil
}

// GenerateQuery calls the GraphQL query-generation endpoint through the
// response-enabled client.
func (c *ClientWithResponses) GenerateQuery(ctx context.Context, orgID int64, body GenerateQueryRequestBody, reqEditors ...RequestEditorFn) (*GenerateQueryResponse, error) {
	client, ok := c.ClientInterface.(*Client)
	if !ok {
		return nil, fmt.Errorf("generate query requires the generated Client implementation")
	}
	return client.GenerateQuery(ctx, orgID, body, reqEditors...)
}
