package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

const (
	generatedPrefix = "client_gen_"
	manifestName    = "client_gen_manifest.json"
)

type options struct {
	input      string
	outputDir  string
	spec       string
	specConfig string
}

type specConfig struct {
	Specs []struct {
		ID             string `yaml:"id"`
		Service        string `yaml:"service"`
		Vendor         string `yaml:"vendor"`
		IncludeInMerge bool   `yaml:"include_in_merge"`
	} `yaml:"specs"`
}

type namespace struct {
	Name     string
	FileSlug string
	SpecID   string
}

type groupKey struct {
	Namespace string
	Concern   string
}

type outputGroup struct {
	Key          groupKey
	FileName     string
	SourceSpecID string
	Decls        []ast.Decl
	Declarations []string
}

type manifest struct {
	FormatVersion int            `json:"formatVersion"`
	Source        string         `json:"source"`
	SourceSHA256  string         `json:"sourceSha256"`
	Generator     string         `json:"generator"`
	Package       string         `json:"package"`
	Files         []manifestFile `json:"files"`
	TotalFiles    int            `json:"totalFiles"`
	TotalLines    int            `json:"totalLines"`
	TotalBytes    int            `json:"totalBytes"`
	TotalDecls    int            `json:"totalDeclarations"`
}

type manifestFile struct {
	Path             string   `json:"path"`
	SHA256           string   `json:"sha256"`
	Namespace        string   `json:"namespace"`
	SourceSpecID     string   `json:"sourceSpecId,omitempty"`
	Concern          string   `json:"concern"`
	Lines            int      `json:"lines"`
	Bytes            int      `json:"bytes"`
	Declarations     []string `json:"declarations"`
	DeclarationCount int      `json:"declarationCount"`
}

func main() {
	var opts options
	flag.StringVar(&opts.input, "input", "", "monolithic generated Go file to split")
	flag.StringVar(&opts.outputDir, "output-dir", ".", "directory for split generated files")
	flag.StringVar(&opts.spec, "spec", "", "OpenAPI document used to generate the client")
	flag.StringVar(&opts.specConfig, "spec-config", "", "specs.yaml used to map namespaces to source specs")
	flag.Parse()

	if err := run(opts); err != nil {
		fmt.Fprintf(os.Stderr, "split-client: %v\n", err)
		os.Exit(1)
	}
}

func run(opts options) error {
	if opts.input == "" {
		return errors.New("--input is required")
	}
	if opts.spec == "" {
		return errors.New("--spec is required")
	}

	namespaces, err := loadNamespaces(opts.specConfig)
	if err != nil {
		return err
	}
	groups, packageName, generator, err := splitFile(opts.input, namespaces)
	if err != nil {
		return err
	}
	return writeOutputs(opts, packageName, generator, groups)
}

func loadNamespaces(path string) ([]namespace, error) {
	byName := map[string]namespace{
		"Auth": {Name: "Auth", FileSlug: "auth", SpecID: "flexera-okta-token-service"},
	}
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read spec config: %w", err)
		}
		var config specConfig
		if err := yaml.Unmarshal(data, &config); err != nil {
			return nil, fmt.Errorf("parse spec config: %w", err)
		}
		for _, spec := range config.Specs {
			if spec.Vendor != "flexera" && !spec.IncludeInMerge {
				continue
			}
			name := componentNamespace(spec.Service)
			entry := namespace{Name: name, FileSlug: snakeCase(name), SpecID: spec.ID}
			if previous, exists := byName[name]; exists && previous.SpecID != entry.SpecID {
				return nil, fmt.Errorf("namespace %q is shared by specs %q and %q", name, previous.SpecID, entry.SpecID)
			}
			byName[name] = entry
		}
	}

	namespaces := make([]namespace, 0, len(byName))
	for _, entry := range byName {
		namespaces = append(namespaces, entry)
	}
	sort.Slice(namespaces, func(i, j int) bool {
		if len(namespaces[i].Name) != len(namespaces[j].Name) {
			return len(namespaces[i].Name) > len(namespaces[j].Name)
		}
		return namespaces[i].Name < namespaces[j].Name
	})
	return namespaces, nil
}

func splitFile(path string, namespaces []namespace) ([]outputGroup, string, string, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, "", "", fmt.Errorf("read generated client: %w", err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, source, parser.ParseComments)
	if err != nil {
		return nil, "", "", fmt.Errorf("parse generated client: %w", err)
	}

	generator := detectGenerator(source)
	packageHeader := strings.TrimSpace(string(source[:int(file.Package)-1]))
	commentMap := ast.NewCommentMap(fset, file, file.Comments)
	imports := collectImports(file)
	groupMap := map[groupKey]*outputGroup{}
	sourceInventory, err := declarationInventory(file.Decls)
	if err != nil {
		return nil, "", "", fmt.Errorf("inventory generated client: %w", err)
	}
	outputInventory := map[string]string{}

	for _, decl := range file.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.IMPORT {
			continue
		}
		for _, part := range splitDeclaration(decl, namespaces) {
			entry, concern, ok := classifyDeclaration(part, namespaces)
			if !ok {
				return nil, "", "", fmt.Errorf("cannot classify declaration %q", declarationName(part))
			}
			key := groupKey{Namespace: entry.Name, Concern: concern}
			group := groupMap[key]
			if group == nil {
				group = &outputGroup{
					Key:          key,
					FileName:     outputFileName(entry, concern),
					SourceSpecID: entry.SpecID,
				}
				groupMap[key] = group
			}
			group.Decls = append(group.Decls, part)
			group.Declarations = append(group.Declarations, declarationNames(part)...)
		}
	}

	groups := make([]outputGroup, 0, len(groupMap))
	for _, group := range groupMap {
		header := fmt.Sprintf("// Code generated by %s and cmd/split-client. DO NOT EDIT.", generator)
		isCore := group.Key.Namespace == "core"
		if isCore {
			header = packageHeader
		}
		rendered, err := renderGroup(fset, file.Name.Name, header, isCore, imports, commentMap, group.Decls)
		if err != nil {
			return nil, "", "", fmt.Errorf("render %s: %w", group.FileName, err)
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), group.FileName, rendered, parser.SkipObjectResolution|parser.ParseComments)
		if err != nil {
			return nil, "", "", fmt.Errorf("validate %s: %w", group.FileName, err)
		}
		inventory, err := declarationInventory(parsed.Decls)
		if err != nil {
			return nil, "", "", fmt.Errorf("inventory %s: %w", group.FileName, err)
		}
		for name, fingerprint := range inventory {
			if _, exists := outputInventory[name]; exists {
				return nil, "", "", fmt.Errorf("declaration %q appears in multiple output files", name)
			}
			outputInventory[name] = fingerprint
		}
		group.Decls = []ast.Decl{&renderedDecl{content: rendered}}
		sort.Strings(group.Declarations)
		groups = append(groups, *group)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].FileName < groups[j].FileName })
	if err := verifyDeclarationInventory(sourceInventory, outputInventory); err != nil {
		return nil, "", "", err
	}
	return groups, file.Name.Name, generator, nil
}

// renderedDecl carries already formatted output through the write phase.
type renderedDecl struct {
	ast.BadDecl
	content []byte
}

func splitDeclaration(decl ast.Decl, namespaces []namespace) []ast.Decl {
	gen, ok := decl.(*ast.GenDecl)
	if !ok || (gen.Tok != token.TYPE && gen.Tok != token.CONST && gen.Tok != token.VAR) || len(gen.Specs) <= 1 {
		return []ast.Decl{decl}
	}

	var commonKey groupKey
	allSame := true
	for index, spec := range gen.Specs {
		part := *gen
		part.Specs = []ast.Spec{spec}
		entry, concern, ok := classifyDeclaration(&part, namespaces)
		if !ok {
			allSame = false
			break
		}
		key := groupKey{Namespace: entry.Name, Concern: concern}
		if index == 0 {
			commonKey = key
		} else if key != commonKey {
			allSame = false
			break
		}
	}
	if allSame {
		return []ast.Decl{decl}
	}

	result := make([]ast.Decl, 0, len(gen.Specs))
	for _, spec := range gen.Specs {
		part := *gen
		part.Specs = []ast.Spec{spec}
		part.Lparen = token.NoPos
		part.Rparen = token.NoPos
		result = append(result, &part)
	}
	return result
}

func classifyDeclaration(decl ast.Decl, namespaces []namespace) (namespace, string, bool) {
	name := classificationName(decl)
	receiver := receiverName(decl)

	if entry, ok := matchNamespace(name, namespaces); ok {
		return entry, concernFor(name, receiver), true
	}
	if entry, ok := matchNamespace(receiver, namespaces); ok {
		return entry, concernFor(name, receiver), true
	}
	if isSharedDeclaration(name, receiver) {
		return namespace{Name: "core", FileSlug: "core"}, "core", true
	}
	return namespace{}, "", false
}

func classificationName(decl ast.Decl) string {
	gen, ok := decl.(*ast.GenDecl)
	if !ok || len(gen.Specs) == 0 {
		return declarationName(decl)
	}
	if len(gen.Specs) > 1 {
		first := *gen
		first.Specs = []ast.Spec{gen.Specs[0]}
		return classificationName(&first)
	}
	value, ok := gen.Specs[0].(*ast.ValueSpec)
	if !ok {
		return declarationName(decl)
	}
	if name := expressionName(value.Type); name != "" {
		return name
	}
	if len(value.Values) == 1 {
		if name := expressionName(value.Values[0]); name != "" {
			return name
		}
	}
	return declarationName(decl)
}

func expressionName(expr ast.Expr) string {
	switch value := expr.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.CallExpr:
		return expressionName(value.Fun)
	case *ast.CompositeLit:
		return expressionName(value.Type)
	case *ast.UnaryExpr:
		return expressionName(value.X)
	default:
		return ""
	}
}

func concernFor(name, receiver string) string {
	if strings.HasPrefix(name, "Parse") ||
		strings.Contains(name, "Response") ||
		strings.Contains(receiver, "Response") ||
		strings.HasSuffix(name, "WithResponse") {
		return "responses"
	}
	if receiver == "Client" || strings.HasPrefix(name, "New") && strings.Contains(name, "Request") ||
		strings.HasSuffix(name, "Params") ||
		strings.Contains(name, "RequestBody") {
		return "operations"
	}
	return "models"
}

func isSharedDeclaration(name, receiver string) bool {
	if receiver == "Client" || receiver == "ClientWithResponses" {
		return true
	}
	switch name {
	case "Client",
		"ClientInterface",
		"ClientOption",
		"ClientWithResponses",
		"ClientWithResponsesInterface",
		"FlexeraBearerAuthScopes",
		"HttpRequestDoer",
		"NewClient",
		"NewClientWithResponses",
		"NewServerUrlFlexeraOneAPI",
		"RequestEditorFn",
		"ServerUrlFlexeraOneAPITestStaging",
		"ServerUrlFlexeraOneAPIZoneVariable",
		"ServerUrlFlexeraOneAPIZoneVariableAu",
		"ServerUrlFlexeraOneAPIZoneVariableCom",
		"ServerUrlFlexeraOneAPIZoneVariableDefault",
		"ServerUrlFlexeraOneAPIZoneVariableEu",
		"WithBaseURL",
		"WithHTTPClient",
		"WithRequestEditorFn":
		return true
	default:
		return false
	}
}

func matchNamespace(name string, namespaces []namespace) (namespace, bool) {
	name = strings.TrimPrefix(name, "New")
	name = strings.TrimPrefix(name, "Parse")
	for _, entry := range namespaces {
		if !strings.HasPrefix(name, entry.Name) {
			continue
		}
		remainder := strings.TrimPrefix(name, entry.Name)
		// An uppercase (or empty) remainder marks a Go identifier word boundary;
		// a lowercase remainder means the namespace is only a textual prefix.
		if remainder == "" || !unicode.IsLower([]rune(remainder)[0]) {
			return entry, true
		}
	}
	return namespace{}, false
}

func declarationName(decl ast.Decl) string {
	names := declarationNames(decl)
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

func declarationNames(decl ast.Decl) []string {
	switch value := decl.(type) {
	case *ast.FuncDecl:
		return []string{value.Name.Name}
	case *ast.GenDecl:
		var names []string
		for _, spec := range value.Specs {
			switch typed := spec.(type) {
			case *ast.TypeSpec:
				names = append(names, typed.Name.Name)
			case *ast.ValueSpec:
				for _, name := range typed.Names {
					names = append(names, name.Name)
				}
			}
		}
		return names
	default:
		return nil
	}
}

func declarationInventory(decls []ast.Decl) (map[string]string, error) {
	inventory := map[string]string{}
	for _, decl := range decls {
		gen, isGen := decl.(*ast.GenDecl)
		if isGen && gen.Tok == token.IMPORT {
			continue
		}

		switch value := decl.(type) {
		case *ast.FuncDecl:
			key := "func:" + receiverName(value) + "." + value.Name.Name
			if err := addFingerprint(inventory, key, value); err != nil {
				return nil, err
			}
		case *ast.GenDecl:
			for _, spec := range value.Specs {
				part := *value
				part.Specs = []ast.Spec{spec}
				switch typed := spec.(type) {
				case *ast.TypeSpec:
					if err := addFingerprint(inventory, "type:"+typed.Name.Name, &part); err != nil {
						return nil, err
					}
				case *ast.ValueSpec:
					for _, name := range typed.Names {
						if err := addFingerprint(inventory, strings.ToLower(value.Tok.String())+":"+name.Name, &part); err != nil {
							return nil, err
						}
					}
				default:
					return nil, fmt.Errorf("unsupported %s declaration spec %T", value.Tok, spec)
				}
			}
		default:
			return nil, fmt.Errorf("unsupported declaration %T", decl)
		}
	}
	return inventory, nil
}

func addFingerprint(inventory map[string]string, key string, node ast.Node) error {
	if _, exists := inventory[key]; exists {
		return fmt.Errorf("duplicate declaration key %q", key)
	}
	var output bytes.Buffer
	if err := ast.Fprint(&output, token.NewFileSet(), node, fingerprintField); err != nil {
		return fmt.Errorf("fingerprint %s: %w", key, err)
	}
	inventory[key] = output.String()
	return nil
}

func fingerprintField(name string, value reflect.Value) bool {
	switch name {
	case "Obj", "Scope", "Unresolved":
		return false
	}
	return value.Type() != reflect.TypeOf(token.Pos(0))
}

func verifyDeclarationInventory(source, output map[string]string) error {
	for name, sourceFingerprint := range source {
		outputFingerprint, exists := output[name]
		if !exists {
			return fmt.Errorf("declaration %q was dropped from split output", name)
		}
		if outputFingerprint != sourceFingerprint {
			return fmt.Errorf("declaration %q changed while splitting", name)
		}
	}
	for name := range output {
		if _, exists := source[name]; !exists {
			return fmt.Errorf("split output introduced declaration %q", name)
		}
	}
	return nil
}

func receiverName(decl ast.Decl) string {
	fn, ok := decl.(*ast.FuncDecl)
	if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	expr := fn.Recv.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

type importInfo struct {
	Spec *ast.ImportSpec
	Name string
}

func collectImports(file *ast.File) []importInfo {
	var result []importInfo
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		name := filepath.Base(path)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		cloned := &ast.ImportSpec{Path: &ast.BasicLit{Kind: token.STRING, Value: spec.Path.Value}}
		if spec.Name != nil {
			cloned.Name = ast.NewIdent(spec.Name.Name)
		}
		result = append(result, importInfo{Spec: cloned, Name: name})
	}
	return result
}

func renderGroup(fset *token.FileSet, packageName, header string, packageDoc bool, imports []importInfo, comments ast.CommentMap, decls []ast.Decl) ([]byte, error) {
	used := usedImportNames(decls)
	file := &ast.File{Name: ast.NewIdent(packageName), Decls: append([]ast.Decl(nil), decls...)}
	var importSpecs []ast.Spec
	for _, imp := range imports {
		if imp.Name != "_" && imp.Name != "." && !used[imp.Name] {
			continue
		}
		importSpecs = append(importSpecs, imp.Spec)
	}
	if len(importSpecs) > 0 {
		file.Decls = append([]ast.Decl{&ast.GenDecl{
			Tok:    token.IMPORT,
			Lparen: token.Pos(1),
			Specs:  importSpecs,
		}}, file.Decls...)
	}
	file.Comments = comments.Filter(file).Comments()

	var body bytes.Buffer
	if err := format.Node(&body, fset, file); err != nil {
		return nil, err
	}
	separator := "\n\n"
	if packageDoc {
		separator = "\n"
	}
	return append([]byte(header+separator), body.Bytes()...), nil
}

func usedImportNames(decls []ast.Decl) map[string]bool {
	used := map[string]bool{}
	for _, decl := range decls {
		ast.Inspect(decl, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if ident, ok := selector.X.(*ast.Ident); ok {
				used[ident.Name] = true
			}
			return true
		})
	}
	return used
}

func writeOutputs(opts options, packageName, generator string, groups []outputGroup) error {
	specData, err := os.ReadFile(opts.spec)
	if err != nil {
		return fmt.Errorf("read source spec: %w", err)
	}
	sum := sha256.Sum256(specData)
	sourcePath, err := filepath.Rel(opts.outputDir, opts.spec)
	if err != nil {
		return fmt.Errorf("make source spec path relative: %w", err)
	}
	result := manifest{
		FormatVersion: 1,
		Source:        filepath.ToSlash(sourcePath),
		SourceSHA256:  hex.EncodeToString(sum[:]),
		Generator:     generator,
		Package:       packageName,
	}

	optimaPrefixes, err := computeOptimaHostedPrefixes(specData)
	if err != nil {
		return fmt.Errorf("derive Optima-hosted path prefixes: %w", err)
	}
	optimaRoutingSource, err := renderOptimaRoutingFile(packageName, generator, optimaPrefixes)
	if err != nil {
		return fmt.Errorf("render optima routing file: %w", err)
	}

	tempDir, err := os.MkdirTemp(opts.outputDir, ".split-client-")
	if err != nil {
		return fmt.Errorf("create temporary output directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	const optimaRoutingFileName = generatedPrefix + "optima_routing.go"
	if err := os.WriteFile(filepath.Join(tempDir, optimaRoutingFileName), optimaRoutingSource, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", optimaRoutingFileName, err)
	}
	optimaSum := sha256.Sum256(optimaRoutingSource)
	optimaLines := bytes.Count(optimaRoutingSource, []byte("\n"))
	result.Files = append(result.Files, manifestFile{
		Path:             optimaRoutingFileName,
		SHA256:           hex.EncodeToString(optimaSum[:]),
		Namespace:        "meta",
		Concern:          "routing",
		Lines:            optimaLines,
		Bytes:            len(optimaRoutingSource),
		Declarations:     []string{"optimaHostedPathPrefixes"},
		DeclarationCount: 1,
	})
	result.TotalLines += optimaLines
	result.TotalBytes += len(optimaRoutingSource)
	result.TotalDecls++

	for _, group := range groups {
		rendered := group.Decls[0].(*renderedDecl).content
		if err := os.WriteFile(filepath.Join(tempDir, group.FileName), rendered, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", group.FileName, err)
		}
		fileSum := sha256.Sum256(rendered)
		lines := bytes.Count(rendered, []byte("\n"))
		result.Files = append(result.Files, manifestFile{
			Path:             group.FileName,
			SHA256:           hex.EncodeToString(fileSum[:]),
			Namespace:        group.Key.Namespace,
			SourceSpecID:     group.SourceSpecID,
			Concern:          group.Key.Concern,
			Lines:            lines,
			Bytes:            len(rendered),
			Declarations:     group.Declarations,
			DeclarationCount: len(group.Declarations),
		})
		result.TotalLines += lines
		result.TotalBytes += len(rendered)
		result.TotalDecls += len(group.Declarations)
	}
	result.TotalFiles = len(result.Files)

	manifestData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	manifestData = append(manifestData, '\n')
	if err := os.WriteFile(filepath.Join(tempDir, manifestName), manifestData, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	entries, err := os.ReadDir(tempDir)
	if err != nil {
		return fmt.Errorf("read temporary output directory: %w", err)
	}
	desired := make(map[string]bool, len(entries))
	for _, entry := range entries {
		desired[entry.Name()] = true
		if entry.Name() == manifestName {
			continue
		}
		from := filepath.Join(tempDir, entry.Name())
		to := filepath.Join(opts.outputDir, entry.Name())
		if err := os.Rename(from, to); err != nil {
			return fmt.Errorf("install %s: %w", entry.Name(), err)
		}
	}

	stale, err := filepath.Glob(filepath.Join(opts.outputDir, generatedPrefix+"*.go"))
	if err != nil {
		return fmt.Errorf("find stale generated files: %w", err)
	}
	for _, path := range stale {
		if desired[filepath.Base(path)] {
			continue
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove stale generated file %s: %w", path, err)
		}
	}

	if err := os.Rename(filepath.Join(tempDir, manifestName), filepath.Join(opts.outputDir, manifestName)); err != nil {
		return fmt.Errorf("install %s: %w", manifestName, err)
	}
	return nil
}

func outputFileName(entry namespace, concern string) string {
	if entry.Name == "core" {
		return generatedPrefix + "core.go"
	}
	return generatedPrefix + entry.FileSlug + "_" + concern + ".go"
}

func detectGenerator(source []byte) string {
	const marker = "Code generated by "
	for _, line := range bytes.Split(source, []byte("\n")) {
		text := string(line)
		index := strings.Index(text, marker)
		if index == -1 {
			continue
		}
		value := strings.TrimSpace(text[index+len(marker):])
		return strings.TrimSpace(strings.TrimSuffix(value, "DO NOT EDIT."))
	}
	return "oapi-codegen"
}

// openAPIDoc is the minimal shape needed to derive Optima-hosted routing
// prefixes: the document-level default servers plus, per path, any
// path-item-level "servers" override (which OpenAPI 3 defines as taking
// precedence over the document-level default for that path's operations).
//
// Operation-level "servers" overrides (e.g. the login/token endpoint's
// override to login.flexera.{zone}) are intentionally NOT modeled here:
// those are handled by dedicated code (see auth.go's AuthHelper) that
// never goes through the generated *Client's HTTP doer, so they don't need
// runtime request rerouting the way path-hosted operations do.
type openAPIDoc struct {
	Servers []struct {
		URL string `json:"url"`
	} `json:"servers"`
	Paths map[string]struct {
		Servers []struct {
			URL string `json:"url"`
		} `json:"servers"`
	} `json:"paths"`
}

// computeOptimaHostedPrefixes derives the top-level path prefixes (e.g.
// "/bill-analysis") whose operations must be rerouted to a non-default
// backend host at runtime (see optima_routing.go's optimaRoutingDoer),
// by inspecting the merged spec's path-item-level "servers" overrides
// directly rather than relying on a hand-maintained list.
//
// A prefix is only emitted when EVERY path under it carries an override
// to the SAME non-default host; any partial or inconsistent coverage is
// treated as an error so an ambiguous or unexpected spec shape fails
// generation loudly instead of producing silently-wrong routing.
func computeOptimaHostedPrefixes(specData []byte) ([]string, error) {
	var doc openAPIDoc
	if err := json.Unmarshal(specData, &doc); err != nil {
		return nil, fmt.Errorf("parse spec: %w", err)
	}
	if len(doc.Servers) == 0 || strings.TrimSpace(doc.Servers[0].URL) == "" {
		return nil, errors.New("spec has no document-level default server")
	}
	defaultHost := serverHost(doc.Servers[0].URL)

	type prefixInfo struct {
		total      int
		overridden int
		hosts      map[string]bool
	}
	prefixes := map[string]*prefixInfo{}
	for path, item := range doc.Paths {
		prefix := firstPathSegment(path)
		info := prefixes[prefix]
		if info == nil {
			info = &prefixInfo{hosts: map[string]bool{}}
			prefixes[prefix] = info
		}
		info.total++
		if len(item.Servers) == 0 {
			continue
		}
		info.overridden++
		for _, server := range item.Servers {
			if host := serverHost(server.URL); host != defaultHost {
				info.hosts[host] = true
			}
		}
	}

	var result []string
	var problems []string
	for prefix, info := range prefixes {
		if len(info.hosts) == 0 {
			continue // no non-default override under this prefix; nothing to do
		}
		switch {
		case info.overridden != info.total:
			problems = append(problems, fmt.Sprintf(
				"%s: only %d/%d paths carry a non-default servers override (expected all-or-nothing)",
				prefix, info.overridden, info.total))
		case len(info.hosts) > 1:
			problems = append(problems, fmt.Sprintf(
				"%s: paths disagree on override host: %s", prefix, strings.Join(sortedKeys(info.hosts), ", ")))
		default:
			result = append(result, prefix)
		}
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("cannot safely auto-derive Optima routing prefixes; fix the spec/merge_servers "+
			"config or handle these prefixes manually:\n  %s", strings.Join(problems, "\n  "))
	}
	sort.Strings(result)
	return result, nil
}

// serverHost extracts just the scheme-less host (with any port/template
// variables like "{zone}" left intact, e.g. "api.optima{zone}.flexeraeng.com")
// from a server URL, discarding any base path. optimaRoutingDoer only ever
// rewrites scheme+host (the merged spec's operation paths already contain
// the full path shape), so a difference in base path alone does not
// indicate a distinct routing target.
func serverHost(rawURL string) string {
	value := rawURL
	if idx := strings.Index(value, "://"); idx >= 0 {
		value = value[idx+3:]
	}
	if idx := strings.Index(value, "/"); idx >= 0 {
		value = value[:idx]
	}
	return value
}

// firstPathSegment returns the leading "/segment" of an OpenAPI path
// template, e.g. "/bill-analysis" for "/bill-analysis/orgs/{orgId}/...".
func firstPathSegment(path string) string {
	trimmed := strings.TrimPrefix(path, "/")
	if idx := strings.Index(trimmed, "/"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	return "/" + trimmed
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// renderOptimaRoutingFile renders the generated optimaHostedPathPrefixes
// file consumed by optima_routing.go's isOptimaHostedPath. Regenerated by
// cmd/split-client on every `make generate` run, so it can never drift out
// of sync with the committed unified-openapi/openapi3.json the way a
// hand-maintained list could.
func renderOptimaRoutingFile(packageName, generator string, prefixes []string) ([]byte, error) {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "// Code generated by cmd/split-client from unified-openapi/openapi3.json using %s. DO NOT EDIT.\n", generator)
	fmt.Fprintf(&buf, "// Regenerate via `make generate` (see generate_client.sh).\n\n")
	fmt.Fprintf(&buf, "package %s\n\n", packageName)
	buf.WriteString("// optimaHostedPathPrefixes lists the top-level path prefixes whose\n")
	buf.WriteString("// operations carry a path-item \"servers\" override in the merged OpenAPI\n")
	buf.WriteString("// document pointing at a host other than the default gateway server. See\n")
	buf.WriteString("// optima_routing.go's isOptimaHostedPath/optimaRoutingDoer for how this is\n")
	buf.WriteString("// used to reroute matching requests to OptimaBaseURL at runtime.\n")
	if len(prefixes) == 0 {
		buf.WriteString("var optimaHostedPathPrefixes = []string{}\n")
	} else {
		buf.WriteString("var optimaHostedPathPrefixes = []string{\n")
		for _, prefix := range prefixes {
			fmt.Fprintf(&buf, "\t%q,\n", prefix)
		}
		buf.WriteString("}\n")
	}
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("gofmt generated optima routing file: %w", err)
	}
	return formatted, nil
}

func componentNamespace(service string) string {
	parts := strings.FieldsFunc(service, func(r rune) bool {
		return r == '_' || r == '-' || r == ' ' || r == '/'
	})
	var builder strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		runes := []rune(part)
		builder.WriteRune(unicode.ToUpper(runes[0]))
		builder.WriteString(string(runes[1:]))
	}
	if builder.Len() == 0 {
		return "Spec"
	}
	return builder.String()
}

func snakeCase(value string) string {
	var builder strings.Builder
	for index, current := range value {
		if unicode.IsUpper(current) && index > 0 {
			previous := rune(value[index-1])
			var next rune
			if index+1 < len(value) {
				next = rune(value[index+1])
			}
			if unicode.IsLower(previous) || (next != 0 && unicode.IsLower(next)) {
				builder.WriteByte('_')
			}
		}
		builder.WriteRune(unicode.ToLower(current))
	}
	return builder.String()
}
