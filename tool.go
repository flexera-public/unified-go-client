package flexera

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

// Tool is the generic contract for a curated workflow. I and O are the
// tool's input and output types; both should be JSON-serializable so the
// tool can be invoked from CLI/MCP/library dispatchers uniformly.
type Tool[I, O any] interface {
	// Name is a stable, kebab-case identifier (e.g. "anomaly-investigation").
	Name() string
	// Description is a one-line summary shown in --help / MCP listings.
	Description() string
	// Invoke runs the workflow synchronously.
	Invoke(ctx context.Context, in I) (O, error)
}

// JSONHandler is the type-erased entry point a dispatcher invokes. It
// accepts a raw JSON input blob (nil/empty/"null" are treated as the zero
// value of I) and returns a JSON-encoded output blob.
type JSONHandler func(ctx context.Context, raw json.RawMessage) (json.RawMessage, error)

// Wrap converts a typed Tool[I,O] into a JSONHandler. The returned
// handler unmarshals raw → I, calls Invoke, and marshals O → json. A nil
// or "null" raw payload yields the zero-value I (useful for tools whose
// every input field is optional).
func Wrap[I, O any](t Tool[I, O]) JSONHandler {
	return func(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
		var in I
		if len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, &in); err != nil {
				return nil, fmt.Errorf("%s: invalid input json: %w", t.Name(), err)
			}
		}
		out, err := t.Invoke(ctx, in)
		if err != nil {
			return nil, err
		}
		return json.Marshal(out)
	}
}

// Entry is a registry record describing one curated tool. It pairs the
// JSON entry point with the metadata needed by dispatchers to list and
// document the tool.
type Entry struct {
	Name        string
	Description string
	Invoke      JSONHandler
}

// Registry is a name-indexed collection of curated tools. Dispatchers
// (CLI, MCP) consult the registry to route requests by name and to
// produce help/listing output. The zero value is ready to use.
type Registry struct {
	mu      sync.RWMutex
	entries map[string]Entry
}

// Register adds an entry. Re-registering an existing name overwrites it
// (the last writer wins). Empty names are rejected.
func (r *Registry) Register(e Entry) error {
	if e.Name == "" {
		return fmt.Errorf("curated: cannot register tool with empty name")
	}
	if e.Invoke == nil {
		return fmt.Errorf("curated: tool %q has nil Invoke", e.Name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.entries == nil {
		r.entries = make(map[string]Entry)
	}
	r.entries[e.Name] = e
	return nil
}

// MustRegister panics on error; convenient for init-time registration.
func (r *Registry) MustRegister(e Entry) {
	if err := r.Register(e); err != nil {
		panic(err)
	}
}

// Lookup returns the entry for name, or false if not registered.
func (r *Registry) Lookup(name string) (Entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[name]
	return e, ok
}

// List returns all entries sorted by name. Safe to call concurrently.
func (r *Registry) List() []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Entry, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
