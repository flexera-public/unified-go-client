package flexera

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Project is the resolver-facing project view. It is a lightweight subset of
// the generated GrsProject DTO (Id + Name) which is all callers historically
// needed for tag/label resolution.
type Project struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// ProjectResolverOption configures a ProjectResolver.
type ProjectResolverOption func(*ProjectResolver)

// WithProjectsCacheTTL sets the cache time-to-live. A zero or negative value
// disables expiry (entries are cached for the lifetime of the resolver).
func WithProjectsCacheTTL(ttl time.Duration) ProjectResolverOption {
	return func(r *ProjectResolver) { r.cacheTTL = ttl }
}

// WithProjectsAPIVersion overrides the X-Api-Version header sent to the GRS
// endpoint. Defaults to "2.0".
func WithProjectsAPIVersion(version string) ProjectResolverOption {
	return func(r *ProjectResolver) {
		if v := strings.TrimSpace(version); v != "" {
			r.apiVersion = v
		}
	}
}

// ErrNoProjects is returned when the caller's org has no projects.
var ErrNoProjects = errors.New("no projects found for organization")

// ErrAmbiguousProject is returned by ResolveSingleProjectID when more than one
// project is available for the organization.
var ErrAmbiguousProject = errors.New("organization has multiple projects; explicit project ID required")

// ErrProjectNotFound is returned by lookup helpers when no matching project is found.
var ErrProjectNotFound = errors.New("project not found")

// ProjectResolver resolves Flexera One project IDs for a given organization
// by calling the legacy GRS user-projects endpoint:
// GET /grs/users/{userId}/projects.
//
// The user ID is decoded from the caller's JWT access token (sub=u-{id} or
// legacy user claim). Service-account tokens are not supported for this
// lookup and return an explicit error.
//
// Lookups are cached per-org with an optional TTL.
type ProjectResolver struct {
	client     *ClientWithResponses
	cacheTTL   time.Duration
	apiVersion string

	mu    sync.RWMutex
	cache map[int64]projectCacheEntry
}

type projectCacheEntry struct {
	projects []Project
	fetched  time.Time
}

// NewProjectResolver constructs a ProjectResolver bound to a generated
// unified ClientWithResponses.
func NewProjectResolver(client *ClientWithResponses, opts ...ProjectResolverOption) (*ProjectResolver, error) {
	if client == nil {
		return nil, errors.New("client is required")
	}
	r := &ProjectResolver{
		client:     client,
		apiVersion: "2.0",
		cache:      map[int64]projectCacheEntry{},
	}
	for _, opt := range opts {
		opt(r)
	}
	return r, nil
}

func zoneFromAPIBaseURL(server string) (Zone, bool) {
	serverURL, err := url.Parse(strings.TrimSpace(server))
	if err != nil || serverURL.Host == "" {
		return "", false
	}

	for _, zone := range canonicalZones {
		apiBaseURL, err := APIBaseURLForZone(zone)
		if err != nil {
			continue
		}
		apiURL, err := url.Parse(apiBaseURL)
		if err != nil {
			continue
		}
		if strings.EqualFold(serverURL.Host, apiURL.Host) {
			return zone, true
		}
	}

	return "", false
}

// ListProjects returns the projects for the given organization. Results are
// cached per-org (subject to the configured TTL). Per-call request editors
// are forwarded to the generated client.
func (r *ProjectResolver) ListProjects(ctx context.Context, orgID int64, editors ...RequestEditorFn) ([]Project, error) {
	if orgID <= 0 {
		return nil, fmt.Errorf("orgID must be positive, got %d", orgID)
	}

	if cached, ok := r.lookupCache(orgID); ok {
		return cached, nil
	}

	projects, err := r.fetch(ctx, orgID, editors)
	if err != nil {
		return nil, err
	}

	r.storeCache(orgID, projects)
	return projects, nil
}

// GetProjectByID returns the project with the matching ID for the org.
func (r *ProjectResolver) GetProjectByID(ctx context.Context, orgID, projectID int64, editors ...RequestEditorFn) (*Project, error) {
	projects, err := r.ListProjects(ctx, orgID, editors...)
	if err != nil {
		return nil, err
	}
	for i := range projects {
		if projects[i].ID == projectID {
			return &projects[i], nil
		}
	}
	return nil, fmt.Errorf("%w: project id %d in org %d", ErrProjectNotFound, projectID, orgID)
}

// GetProjectByName returns the first project whose name matches (case-insensitive).
func (r *ProjectResolver) GetProjectByName(ctx context.Context, orgID int64, name string, editors ...RequestEditorFn) (*Project, error) {
	want := strings.TrimSpace(strings.ToLower(name))
	if want == "" {
		return nil, errors.New("project name is required")
	}
	projects, err := r.ListProjects(ctx, orgID, editors...)
	if err != nil {
		return nil, err
	}
	for i := range projects {
		if strings.EqualFold(strings.TrimSpace(projects[i].Name), name) {
			return &projects[i], nil
		}
	}
	return nil, fmt.Errorf("%w: name %q in org %d", ErrProjectNotFound, name, orgID)
}

// ResolveSingleProjectID returns the only project's ID for the org, or an
// error if the org contains zero or more than one project.
func (r *ProjectResolver) ResolveSingleProjectID(ctx context.Context, orgID int64, editors ...RequestEditorFn) (int64, error) {
	projects, err := r.ListProjects(ctx, orgID, editors...)
	if err != nil {
		return 0, err
	}
	switch len(projects) {
	case 0:
		return 0, fmt.Errorf("%w: org %d", ErrNoProjects, orgID)
	case 1:
		return projects[0].ID, nil
	default:
		return 0, fmt.Errorf("%w: org %d has %d projects", ErrAmbiguousProject, orgID, len(projects))
	}
}

// AutoDetectProjectID returns the first project's ID. The second return
// value reports whether the org contains more than one project.
func (r *ProjectResolver) AutoDetectProjectID(ctx context.Context, orgID int64, editors ...RequestEditorFn) (id int64, ambiguous bool, err error) {
	projects, err := r.ListProjects(ctx, orgID, editors...)
	if err != nil {
		return 0, false, err
	}
	if len(projects) == 0 {
		return 0, false, fmt.Errorf("%w: org %d", ErrNoProjects, orgID)
	}
	return projects[0].ID, len(projects) > 1, nil
}

// Invalidate clears the cached entry for an org.
func (r *ProjectResolver) Invalidate(orgID int64) {
	r.mu.Lock()
	delete(r.cache, orgID)
	r.mu.Unlock()
}

// InvalidateAll clears the entire cache.
func (r *ProjectResolver) InvalidateAll() {
	r.mu.Lock()
	r.cache = map[int64]projectCacheEntry{}
	r.mu.Unlock()
}

func (r *ProjectResolver) lookupCache(orgID int64) ([]Project, bool) {
	r.mu.RLock()
	entry, ok := r.cache[orgID]
	r.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if r.cacheTTL > 0 && time.Since(entry.fetched) > r.cacheTTL {
		return nil, false
	}
	return entry.projects, true
}

func (r *ProjectResolver) storeCache(orgID int64, projects []Project) {
	r.mu.Lock()
	r.cache[orgID] = projectCacheEntry{projects: projects, fetched: time.Now()}
	r.mu.Unlock()
}

func (r *ProjectResolver) fetch(ctx context.Context, orgID int64, editors []RequestEditorFn) ([]Project, error) {
	baseClient, ok := r.client.ClientInterface.(*Client)
	if !ok || baseClient == nil || baseClient.Client == nil {
		return nil, errors.New("project resolver requires a concrete unified client with an HTTP doer")
	}

	grsBaseURL := strings.TrimRight(strings.TrimSpace(baseClient.Server), "/")
	if zone, ok := zoneFromAPIBaseURL(baseClient.Server); ok {
		if mapped := strings.TrimSpace(GrsBaseURLForZone(zone)); mapped != "" {
			grsBaseURL = mapped
		}
	}

	token, err := accessTokenFromClient(ctx, baseClient, editors)
	if err != nil {
		return nil, err
	}
	userID, err := UserIDFromToken(token)
	if err != nil {
		return nil, fmt.Errorf("derive user ID from access token: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fmt.Sprintf("%s/grs/users/%d/projects", strings.TrimRight(grsBaseURL, "/"), userID),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("build user projects request: %w", err)
	}
	req.Header.Set("X-Api-Version", r.apiVersion)

	if err := applyRequestEditors(ctx, req, baseClient.RequestEditors, editors); err != nil {
		return nil, fmt.Errorf("apply request editors: %w", err)
	}

	httpResp, err := baseClient.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call user projects endpoint: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		var errBody json.RawMessage
		_ = json.NewDecoder(httpResp.Body).Decode(&errBody)
		return nil, fmt.Errorf("projects request failed: %w", ResponseError(httpResp.StatusCode, errBody))
	}

	var raw []struct {
		ID     int64  `json:"id"`
		Name   string `json:"name"`
		Legacy struct {
			AccountID int64 `json:"account_id"`
		} `json:"legacy"`
		Links struct {
			Org struct {
				ID int64 `json:"id"`
			} `json:"org"`
		} `json:"links"`
	}
	if err := json.NewDecoder(httpResp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode projects response: %w", err)
	}

	out := make([]Project, 0, len(raw))
	for _, p := range raw {
		if p.Links.Org.ID != 0 && p.Links.Org.ID != orgID {
			continue
		}
		projectID := p.Legacy.AccountID
		if projectID <= 0 {
			projectID = p.ID
		}
		if projectID <= 0 {
			continue
		}
		out = append(out, Project{ID: projectID, Name: p.Name})
	}
	return out, nil
}

func accessTokenFromClient(ctx context.Context, client *Client, additionalEditors []RequestEditorFn) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.invalid/", nil)
	if err != nil {
		return "", err
	}
	if err := applyRequestEditors(ctx, req, client.RequestEditors, additionalEditors); err != nil {
		return "", err
	}
	if token := bearerToken(req.Header.Get("Authorization")); token != "" {
		return token, nil
	}

	type accessTokenRefresher interface {
		AccessToken() string
		Refresh(context.Context) error
	}
	if doer, ok := unwrapTokenSourceDoer(client.Client).(accessTokenRefresher); ok {
		if tok := strings.TrimSpace(doer.AccessToken()); tok != "" {
			return tok, nil
		}
		if err := doer.Refresh(ctx); err != nil {
			return "", fmt.Errorf("refresh access token: %w", err)
		}
		if tok := strings.TrimSpace(doer.AccessToken()); tok != "" {
			return tok, nil
		}
	}

	return "", errors.New("unable to determine access token for project lookup")
}

func unwrapTokenSourceDoer(doer HttpRequestDoer) HttpRequestDoer {
	switch d := doer.(type) {
	case *retryingDoer:
		return unwrapTokenSourceDoer(d.next)
	case *debugDoer:
		return unwrapTokenSourceDoer(d.inner)
	default:
		return doer
	}
}

func applyRequestEditors(ctx context.Context, req *http.Request, baseEditors []RequestEditorFn, additionalEditors []RequestEditorFn) error {
	for _, editor := range baseEditors {
		if err := editor(ctx, req); err != nil {
			return err
		}
	}
	for _, editor := range additionalEditors {
		if err := editor(ctx, req); err != nil {
			return err
		}
	}
	return nil
}

func bearerToken(authz string) string {
	const prefix = "Bearer "
	if strings.HasPrefix(authz, prefix) {
		return strings.TrimSpace(strings.TrimPrefix(authz, prefix))
	}
	return ""
}
