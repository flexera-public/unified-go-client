package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestTableResponseKinds(t *testing.T) {
	models := parseTableFixture(t, "models.go", `package flexera
type FirstKind string
const (
	FirstValue FirstKind = "first"
	Other FirstKind = "other"
	Repeated
)
type First struct { Kind FirstKind `+"`json:\"kind\"`"+` }
type SecondKind string
const Shared SecondKind = "shared"
type Second struct { Kind *SecondKind `+"`json:\"kind,omitempty\"`"+` }
type ThirdKind string
const AlsoShared ThirdKind = "shared"
type Third struct { Kind ThirdKind `+"`json:\"kind\"`"+` }
type NoKind struct { Values []string }
type Array = []string
type UnsupportedKind string
const UnsupportedValue UnsupportedKind = "unsupported"
type Unsupported struct { Kind UnsupportedKind `+"`json:\"kind\"`"+` }
`)
	renderer := parseTableFixture(t, "render_table.go", `package flexera
func WriteTable(value any) {
	switch value.(type) {
	case *First:
	case *Second:
	case *Third:
	case *NoKind:
	case *Array:
	case *[]string:
	case map[string]any:
	}
}`)
	supported, err := tableResponseTypes(renderer)
	if err != nil {
		t.Fatal(err)
	}
	aliases := map[string]string{"legacy": "First", "unavailable": "Unsupported"}
	got, err := tableResponseKinds(models, supported, aliases)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"first": "First", "other": "First", "legacy": "First"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dispatch = %#v, want %#v", got, want)
	}
	first, err := renderTableFile("flexera", models, renderer, aliases)
	if err != nil {
		t.Fatal(err)
	}
	// Repeated rendering must not depend on map iteration order.
	for i := 0; i < 10; i++ {
		next, err := renderTableFile("flexera", models, renderer, aliases)
		if err != nil || !bytes.Equal(next, first) {
			t.Fatalf("nondeterministic output: %v\n%s", err, next)
		}
	}
	parsed := parseTableFixture(t, "client_gen_table.go", string(first))
	if parsed.Name.Name != "flexera" || !hasGeneratedHeader(first) {
		t.Fatalf("invalid generated file:\n%s", first)
	}
	for _, conflict := range []map[string]string{
		{"shared": "Second"},
		{"first": "Second"},
	} {
		if _, err := tableResponseKinds(models, supported, conflict); err == nil {
			t.Fatalf("expected alias collision error for %v", conflict)
		}
	}
	supported["Missing"] = true
	if _, err := tableResponseKinds(models, supported, nil); err == nil {
		t.Fatal("expected error for supported response absent from models")
	}
}

func TestTableResponseTypes_RejectsMissingSwitch(t *testing.T) {
	for _, source := range []string{"package flexera", "package flexera; func WriteTable(value any) {}"} {
		if _, err := tableResponseTypes(parseTableFixture(t, "render_table.go", source)); err == nil {
			t.Fatalf("expected error for missing dispatcher: %s", source)
		}
	}
}

func TestRunGeneratesTableDispatch(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "client.gen.tmp.go")
	spec := filepath.Join(dir, "openapi.json")
	config := filepath.Join(dir, "specs.yaml")
	writeTestFile(t, input, generatedFixture+`
type BudgetListKind string
const BudgetListKindValue BudgetListKind = "budget:list"
type BudgetList struct { Kind BudgetListKind `+"`json:\"kind\"`"+` }
`)
	writeTestFile(t, spec, `{"openapi":"3.0.3","servers":[{"url":"https://api.flexera.{zone}"}]}`)
	writeTestFile(t, config, "specs:\n  - id: flexera-budget-v1\n    vendor: flexera\n    service: budget\n")
	rendererPath := filepath.Join(dir, "render_table.go")
	writeTestFile(t, rendererPath, `package flexera
func WriteTable(value any) {
	switch value.(type) { case *BudgetList: }
}
`)
	opts := options{input: input, outputDir: dir, spec: spec, specConfig: config}
	if err := run(opts); err != nil {
		t.Fatal(err)
	}
	first := readTestFile(t, filepath.Join(dir, tableFileName))
	if !strings.Contains(first, `case "budget:list":`) || !strings.Contains(first, "return &BudgetList{}") {
		t.Fatalf("missing generated dispatch:\n%s", first)
	}
	var inventory manifest
	if err := json.Unmarshal([]byte(readTestFile(t, filepath.Join(dir, manifestName))), &inventory); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, file := range inventory.Files {
		if file.Path == tableFileName {
			found = true
			if file.Concern != "table" || !reflect.DeepEqual(file.Declarations, []string{"newTableResponse"}) {
				t.Fatalf("incorrect table inventory: %+v", file)
			}
		}
	}
	if !found {
		t.Fatal("table dispatch absent from generated manifest")
	}
	if err := run(opts); err != nil {
		t.Fatal(err)
	}
	if next := readTestFile(t, filepath.Join(dir, tableFileName)); next != first {
		t.Fatal("regeneration changed table dispatch")
	}
	writeTestFile(t, rendererPath, `package flexera
func WriteTable(value any) {
	switch value.(type) { case map[string]any: }
}
`)
	if err := run(opts); err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, filepath.Join(dir, tableFileName)); strings.Contains(got, `"budget:list"`) {
		t.Fatalf("removed renderer left stale map dispatch:\n%s", got)
	}
}

func TestCommittedTableDispatchIsCurrent(t *testing.T) {
	dir := filepath.Join("..", "..")
	paths, err := filepath.Glob(filepath.Join(dir, generatedPrefix+"*.go"))
	if err != nil {
		t.Fatal(err)
	}
	models := &ast.File{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		models.Decls = append(models.Decls, parseTableFixture(t, path, string(data)).Decls...)
	}
	rendererPath := filepath.Join(dir, "render_table.go")
	renderer := parseTableFixture(t, rendererPath, readTestFile(t, rendererPath))
	want, err := renderTableFile("flexera", models, renderer, tableKindAliases)
	if err != nil {
		t.Fatal(err)
	}
	got := readTestFile(t, filepath.Join(dir, tableFileName))
	if got != string(want) {
		t.Fatal("client_gen_table.go is stale; run make generate-client")
	}
}

func parseTableFixture(t *testing.T, name, source string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), name, source, 0)
	if err != nil {
		t.Fatal(fmt.Errorf("parse %s: %w", name, err))
	}
	return file
}
