package flexera

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Project is the resolver-facing project view.
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

// ErrNoProjects is returned when the caller's org has no projects.
var ErrNoProjects = errors.New("no projects found for organization")

// ErrAmbiguousProject is returned by ResolveSingleProjectID when more than one
// project is available for the organization.
var ErrAmbiguousProject = errors.New("organization has multiple projects; explicit project ID required")

// ErrProjectNotFound is returned by lookup helpers when no matching project is found.
var ErrProjectNotFound = errors.New("project not found")

// ProjectResolver resolves Flexera One project IDs for a given organization
// through the IAM org-projects endpoint. Lookups are cached per-org with an
// optional TTL.
type ProjectResolver struct {
	client   *ClientWithResponses
	cacheTTL time.Duration

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
		client: client,
		cache:  map[int64]projectCacheEntry{},
	}
	for _, opt := range opts {
		opt(r)
	}
	return r, nil
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
	response, err := r.client.IamProjectIndexWithResponse(ctx, int(orgID), nil, editors...)
	if err != nil {
		return nil, fmt.Errorf("call org projects endpoint: %w", err)
	}
	if response.StatusCode() < 200 || response.StatusCode() >= 300 {
		return nil, fmt.Errorf("projects request failed: %w", ResponseError(response.StatusCode(), response.Body))
	}
	if response.JSON200 == nil {
		return nil, errors.New("decode projects response: missing HTTP 200 response body")
	}

	out := make([]Project, 0, len(*response.JSON200))
	for _, p := range *response.JSON200 {
		if p.Id <= 0 {
			continue
		}
		out = append(out, Project{ID: int64(p.Id), Name: p.Name})
	}
	return out, nil
}
