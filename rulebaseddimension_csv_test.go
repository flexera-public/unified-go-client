package flexera

import (
	"strings"
	"testing"
)

func TestGenerateRuleBasedDimensionsFromCSVBasic(t *testing.T) {
	csvData := `vendor,region,%SEPARATOR%,rbd_department,rbd_team
AWS,us-east-1,,Engineering,Platform
Azure,,,Sales,
`
	opts := DefaultRuleBasedDimensionCSVOptions()
	specs, err := GenerateRuleBasedDimensionsFromCSV(strings.NewReader(csvData), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d: %+v", len(specs), specs)
	}
	if specs[0].ID != "rbd_department" || specs[0].Name != "rbd_department" {
		t.Fatalf("unexpected first spec: %+v", specs[0])
	}
	if len(specs[0].Rules) != 2 {
		t.Fatalf("expected 2 rules for rbd_department, got %d", len(specs[0].Rules))
	}
	if len(specs[1].Rules) != 1 {
		t.Fatalf("expected 1 rule for rbd_team (second row has empty output cell), got %d", len(specs[1].Rules))
	}
	// First rule for rbd_department should AND vendor+region conditions.
	cond := specs[0].Rules[0].Condition
	if cond == nil || cond.Type != And || cond.Expressions == nil || len(*cond.Expressions) != 2 {
		t.Fatalf("expected AND of 2 expressions, got %+v", cond)
	}
}

func TestGenerateRuleBasedDimensionsFromCSVConditionSyntax(t *testing.T) {
	csvData := `vendor,%SEPARATOR%,rbd_out
DIMENSION_CONTAINS(comp),,Compute
NOT(DIMENSION_EQUALS(Azure)),,NotAzure
PlainAWS,,PlainMatch
`
	opts := DefaultRuleBasedDimensionCSVOptions()
	specs, err := GenerateRuleBasedDimensionsFromCSV(strings.NewReader(csvData), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	rules := specs[0].Rules
	if len(rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(rules))
	}
	if rules[0].Condition.Type != DimensionContains || rules[0].Condition.Substring == nil || *rules[0].Condition.Substring != "comp" {
		t.Fatalf("unexpected DIMENSION_CONTAINS condition: %+v", rules[0].Condition)
	}
	if rules[1].Condition.Type != Not || rules[1].Condition.Expression == nil || rules[1].Condition.Expression.Type != DimensionEquals {
		t.Fatalf("unexpected NOT condition: %+v", rules[1].Condition)
	}
	if rules[2].Condition.Type != DimensionEquals || rules[2].Condition.Value == nil || *rules[2].Condition.Value != "PlainAWS" {
		t.Fatalf("unexpected plain-value condition: %+v", rules[2].Condition)
	}
}

func TestGenerateRuleBasedDimensionsFromCSVOutputValueSyntax(t *testing.T) {
	csvData := `vendor,%SEPARATOR%,rbd_out
AWS,,{{vendor}}
AWS,,VALUE_FROM_DIMENSION(vendor)
AWS,,StaticText
`
	opts := DefaultRuleBasedDimensionCSVOptions()
	specs, err := GenerateRuleBasedDimensionsFromCSV(strings.NewReader(csvData), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rules := specs[0].Rules
	if len(rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(rules))
	}
	if rules[0].Value.Dimension == nil || *rules[0].Value.Dimension != "vendor" {
		t.Fatalf("expected {{vendor}} to resolve to dimension reference, got %+v", rules[0].Value)
	}
	if rules[1].Value.Dimension == nil || *rules[1].Value.Dimension != "vendor" {
		t.Fatalf("expected VALUE_FROM_DIMENSION(vendor) to resolve to dimension reference, got %+v", rules[1].Value)
	}
	if rules[2].Value.Text == nil || *rules[2].Value.Text != "StaticText" {
		t.Fatalf("expected static text value, got %+v", rules[2].Value)
	}
}

func TestGenerateRuleBasedDimensionsFromCSVCaseInsensitive(t *testing.T) {
	csvData := `vendor,%SEPARATOR%,rbd_out
AWS,,match
`
	opts := DefaultRuleBasedDimensionCSVOptions()
	opts.CaseInsensitive = true
	specs, err := GenerateRuleBasedDimensionsFromCSV(strings.NewReader(csvData), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cond := specs[0].Rules[0].Condition
	if cond.CaseInsensitive == nil || !*cond.CaseInsensitive {
		t.Fatalf("expected case-insensitive flag to be set, got %+v", cond)
	}

	opts.CaseInsensitive = false
	specs, err = GenerateRuleBasedDimensionsFromCSV(strings.NewReader(csvData), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if specs[0].Rules[0].Condition.CaseInsensitive != nil {
		t.Fatalf("expected case-insensitive flag to be unset, got %+v", specs[0].Rules[0].Condition)
	}
}

func TestGenerateRuleBasedDimensionsFromCSVColumnConfig(t *testing.T) {
	csvData := `vendor,%SEPARATOR%,Department,Team
AWS,,Engineering,Platform
`
	opts := DefaultRuleBasedDimensionCSVOptions()
	opts.ColumnConfig = map[string]RuleBasedDimensionCSVColumnOverride{
		"Department": {ID: "rbd_dept_override", Name: "Dept Override"},
		"Team":       {Skip: true},
	}
	specs, err := GenerateRuleBasedDimensionsFromCSV(strings.NewReader(csvData), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec (Team skipped), got %d: %+v", len(specs), specs)
	}
	if specs[0].ID != "rbd_dept_override" || specs[0].Name != "Dept Override" {
		t.Fatalf("unexpected override application: %+v", specs[0])
	}
}

func TestGenerateRuleBasedDimensionsFromCSVDescriptionColumnsSkipped(t *testing.T) {
	csvData := `vendor,%DESCRIPTION%vendor_notes,%SEPARATOR%,rbd_out,%DESCRIPTION%out_notes
AWS,some notes,,Engineering,ignored
`
	opts := DefaultRuleBasedDimensionCSVOptions()
	specs, err := GenerateRuleBasedDimensionsFromCSV(strings.NewReader(csvData), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if len(specs[0].Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(specs[0].Rules))
	}
}

func TestGenerateRuleBasedDimensionsFromCSVErrors(t *testing.T) {
	opts := DefaultRuleBasedDimensionCSVOptions()

	if _, err := GenerateRuleBasedDimensionsFromCSV(strings.NewReader(""), opts); err == nil {
		t.Fatalf("expected error for empty CSV")
	}

	if _, err := GenerateRuleBasedDimensionsFromCSV(strings.NewReader("vendor,rbd_out\nAWS,Engineering\n"), opts); err == nil {
		t.Fatalf("expected error for missing separator header")
	}

	if _, err := GenerateRuleBasedDimensionsFromCSV(strings.NewReader("%SEPARATOR%,rbd_out\n,Engineering\n"), opts); err == nil {
		t.Fatalf("expected error for no rule dimension columns")
	}

	if _, err := GenerateRuleBasedDimensionsFromCSV(strings.NewReader("vendor,%SEPARATOR%\nAWS,\n"), opts); err == nil {
		t.Fatalf("expected error for no output columns")
	}

	if _, err := GenerateRuleBasedDimensionsFromCSV(strings.NewReader("bad name!,%SEPARATOR%,rbd_out\nAWS,,Engineering\n"), opts); err == nil {
		t.Fatalf("expected error for invalid dimension column name")
	}

	// Two output columns that render to the same ID should conflict.
	dup := `vendor,%SEPARATOR%,Foo Bar,Foo-Bar
AWS,,Engineering,Platform
`
	if _, err := GenerateRuleBasedDimensionsFromCSV(strings.NewReader(dup), opts); err == nil {
		t.Fatalf("expected error for duplicate generated ID")
	}

	// Output column not starting with rbd_ after templating should error if
	// the ID template itself doesn't produce the rbd_ prefix.
	badOpts := DefaultRuleBasedDimensionCSVOptions()
	badOpts.IDTemplate = "{{.ColumnSlug}}"
	if _, err := GenerateRuleBasedDimensionsFromCSV(strings.NewReader("vendor,%SEPARATOR%,Department\nAWS,,Engineering\n"), badOpts); err == nil {
		t.Fatalf("expected error for ID missing rbd_ prefix")
	}
}

func TestGenerateRuleBasedDimensionsFromCSVSkipsEmptyRows(t *testing.T) {
	csvData := "vendor,%SEPARATOR%,rbd_out\nAWS,,Engineering\n,,\n"
	opts := DefaultRuleBasedDimensionCSVOptions()
	specs, err := GenerateRuleBasedDimensionsFromCSV(strings.NewReader(csvData), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs[0].Rules) != 1 {
		t.Fatalf("expected empty row to be skipped, got %d rules", len(specs[0].Rules))
	}
}
