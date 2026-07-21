package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// These functions extend the generated GraphQL client with additional functionality
// The paths are not defined in the openapi spec, but are available in the API

// GenerateQueryRequestBody defines the request body for query generation
type GenerateQueryRequestBody struct {
	// Prompt is a natural language description for generating a new query
	Prompt string `json:"prompt,omitempty"`

	// Query is the existing GraphQL query to modify (optional)
	Query string `json:"query,omitempty"`

	// ModifyPrompt is a natural language instruction for modifying the existing query
	ModifyPrompt string `json:"modifyPrompt,omitempty"`

	// Indent controls whether the generated query should be indented
	Indent bool `json:"indent,omitempty"`
}

// GenerateQueryResponse defines the response from query generation
type GenerateQueryResponse struct {
	// Prompt is the interpreted prompt explaining what the query does
	Prompt string `json:"prompt"`

	// Query is the generated GraphQL query
	Query string `json:"query"`

	// Valid indicates whether the generated query is valid
	Valid bool `json:"valid"`
}

// GenerateQuery generates or modifies a GraphQL query using natural language
// This endpoint is not in the OpenAPI spec but is available in the API
func (c *Client) GenerateQuery(
	ctx context.Context,
	orgId int64,
	body GenerateQueryRequestBody,
	reqEditors ...RequestEditorFn,
) (*GenerateQueryResponse, error) {
	// Marshal request body
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Build URL
	url := fmt.Sprintf("%s/graphql/v1/orgs/%d/generate", c.Server, orgId)

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")

	// Apply request editors (for authentication, etc.)
	if err := c.applyEditors(ctx, req, reqEditors); err != nil {
		return nil, fmt.Errorf("failed to apply request editors: %w", err)
	}

	// Execute request
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check status code
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var result GenerateQueryResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

// GenerateQuery generates or modifies a GraphQL query using natural language
// This is a wrapper for ClientWithResponses
func (c *ClientWithResponses) GenerateQuery(
	ctx context.Context,
	orgId int64,
	body GenerateQueryRequestBody,
	reqEditors ...RequestEditorFn,
) (*GenerateQueryResponse, error) {
	// Delegate to the Client implementation
	return c.ClientInterface.(*Client).GenerateQuery(ctx, orgId, body, reqEditors...)
}
