package flexera

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// This test enforces the generated-file-header contract documented in
// CONTRIBUTING.md: every generated file (root package and every
// service/*, rightscale/* sub-package) must start with the standard
// "Code generated ... DO NOT EDIT." header, and every hand-written file
// must not. Downstream tooling (flexera-cli's make check-client-coverage)
// relies on exactly this pairing to classify exported declarations without
// re-deriving it from OpenAPI spec diffs, so a drift here would silently
// break that classification.
//
// "Generated" is independently identified here by filename convention
// (client_gen_*.go in the root package, client.gen.go in sub-packages)
// rather than by the header itself, so this test can actually catch a
// missing/extra header instead of trivially agreeing with it.
var generatedHeaderPattern = regexp.MustCompile(`^//\s*Code generated .* DO NOT EDIT\.\s*$`)

func TestGeneratedFileHeaderContract(t *testing.T) {
	skipDirs := map[string]bool{
		".git":    true,
		"cmd":     true, // build tooling, not part of the client packages
		"scripts": true,
	}

	var offenders []string
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] && path != "." {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}

		isGeneratedName := name == "client.gen.go" || strings.HasPrefix(name, "client_gen_")
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		hasHeader := hasGeneratedFileHeader(source)

		switch {
		case isGeneratedName && !hasHeader:
			offenders = append(offenders, path+": named as generated but missing the \"Code generated ... DO NOT EDIT.\" header")
		case !isGeneratedName && hasHeader:
			offenders = append(offenders, path+": has a \"Code generated ... DO NOT EDIT.\" header but is not a recognized generated-file name")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking repository: %v", err)
	}
	if len(offenders) > 0 {
		t.Fatalf("generated-file-header contract violated (see CONTRIBUTING.md):\n%s", strings.Join(offenders, "\n"))
	}
}

// hasGeneratedFileHeader reports whether source starts with the standard
// "Code generated ... DO NOT EDIT." header within its first few lines.
func hasGeneratedFileHeader(source []byte) bool {
	lines := strings.SplitN(string(source), "\n", 6)
	for _, line := range lines {
		if generatedHeaderPattern.MatchString(strings.TrimRight(line, "\r")) {
			return true
		}
	}
	return false
}
