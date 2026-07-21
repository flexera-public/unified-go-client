package flexera

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// echoTool is a trivial test implementation of Tool[I,O].
type echoIn struct {
	Msg string `json:"msg"`
}
type echoOut struct {
	Echo string `json:"echo"`
}
type echoTool struct{}

func (echoTool) Name() string        { return "echo" }
func (echoTool) Description() string { return "echo a message" }
func (echoTool) Invoke(_ context.Context, in echoIn) (echoOut, error) {
	return echoOut{Echo: in.Msg}, nil
}

type failTool struct{}

func (failTool) Name() string        { return "fail" }
func (failTool) Description() string { return "always fails" }
func (failTool) Invoke(context.Context, echoIn) (echoOut, error) {
	return echoOut{}, errors.New("boom")
}

func TestWrap_HappyPath(t *testing.T) {
	h := Wrap[echoIn, echoOut](echoTool{})
	out, err := h(context.Background(), json.RawMessage(`{"msg":"hi"}`))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(string(out), `"hi"`) {
		t.Fatalf("expected echo in output, got %s", out)
	}
}

func TestWrap_NilAndNullInputUseZeroValue(t *testing.T) {
	h := Wrap[echoIn, echoOut](echoTool{})
	for name, raw := range map[string]json.RawMessage{
		"nil":   nil,
		"empty": json.RawMessage(``),
		"null":  json.RawMessage(`null`),
	} {
		t.Run(name, func(t *testing.T) {
			out, err := h(context.Background(), raw)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if string(out) != `{"echo":""}` {
				t.Fatalf("expected zero-value echo, got %s", out)
			}
		})
	}
}

func TestWrap_InvalidJSON(t *testing.T) {
	h := Wrap[echoIn, echoOut](echoTool{})
	_, err := h(context.Background(), json.RawMessage(`{not json}`))
	if err == nil || !strings.Contains(err.Error(), "echo: invalid input json") {
		t.Fatalf("expected named parse error, got %v", err)
	}
}

func TestWrap_InvokeErrorPropagates(t *testing.T) {
	h := Wrap[echoIn, echoOut](failTool{})
	_, err := h(context.Background(), json.RawMessage(`{}`))
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected boom, got %v", err)
	}
}

func TestRegistry_RegisterAndLookup(t *testing.T) {
	var r Registry
	if err := r.Register(Entry{Name: "echo", Description: "d", Invoke: Wrap[echoIn, echoOut](echoTool{})}); err != nil {
		t.Fatalf("register: %v", err)
	}
	e, ok := r.Lookup("echo")
	if !ok || e.Name != "echo" {
		t.Fatalf("lookup failed: ok=%v entry=%+v", ok, e)
	}
	if _, ok := r.Lookup("missing"); ok {
		t.Fatal("expected missing lookup to fail")
	}
}

func TestRegistry_RegisterRejectsEmpty(t *testing.T) {
	var r Registry
	if err := r.Register(Entry{Name: "", Invoke: func(context.Context, json.RawMessage) (json.RawMessage, error) { return nil, nil }}); err == nil {
		t.Fatal("expected error for empty name")
	}
	if err := r.Register(Entry{Name: "x", Invoke: nil}); err == nil {
		t.Fatal("expected error for nil invoke")
	}
}

func TestRegistry_ListIsSorted(t *testing.T) {
	var r Registry
	noop := JSONHandler(func(context.Context, json.RawMessage) (json.RawMessage, error) { return nil, nil })
	for _, n := range []string{"c", "a", "b"} {
		_ = r.Register(Entry{Name: n, Invoke: noop})
	}
	got := r.List()
	if len(got) != 3 || got[0].Name != "a" || got[1].Name != "b" || got[2].Name != "c" {
		t.Fatalf("expected sorted [a,b,c], got %+v", got)
	}
}
