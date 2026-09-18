package flexera

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"text/template"
)

// RuleBasedDimensionCSVDescriptionColumnPrefix marks a CSV column as
// documentation-only; such columns are skipped entirely when generating
// rule-based dimensions.
const RuleBasedDimensionCSVDescriptionColumnPrefix = "%DESCRIPTION%"

// RuleBasedDimensionCSVColumnOverride lets a caller pin an exact ID/Name for
// an output column, or mark it to be skipped, instead of deriving them from
// RuleBasedDimensionCSVOptions' templates.
type RuleBasedDimensionCSVColumnOverride struct {
	ID   string
	Name string
	Skip bool
}

// RuleBasedDimensionCSVTemplateData is the data available to
// RuleBasedDimensionCSVOptions.IDTemplate and NameTemplate.
type RuleBasedDimensionCSVTemplateData struct {
	ColumnHeader string
	ColumnSlug   string
	ColumnIndex  string
}

// RuleBasedDimensionCSVOptions configures GenerateRuleBasedDimensionsFromCSV.
//
// The source CSV has columns to the left of SeparatorHeader describing rule
// dimensions (conditions), and columns to the right describing output RBDs;
// one RuleBasedDimensionSpec is produced per non-skipped output column.
type RuleBasedDimensionCSVOptions struct {
	// SeparatorHeader is the column header separating rule-dimension columns
	// (left) from output columns (right).
	SeparatorHeader string
	// IDTemplate is a text/template rendered with RuleBasedDimensionCSVTemplateData
	// to produce each dimension's ID, unless overridden by ColumnConfig.
	IDTemplate string
	// NameTemplate is a text/template rendered with RuleBasedDimensionCSVTemplateData
	// to produce each dimension's display name, unless overridden by ColumnConfig.
	NameTemplate string
	// EffectiveAt is the effective month (e.g. "2024-01") applied to every
	// generated dimension's rules.
	EffectiveAt string
	// ColumnConfig optionally overrides the ID/Name/Skip behavior for
	// specific output column headers.
	ColumnConfig map[string]RuleBasedDimensionCSVColumnOverride
	// CaseInsensitive marks every generated dimension_equals/dimension_contains
	// condition as case-insensitive.
	CaseInsensitive bool
}

// DefaultRuleBasedDimensionCSVOptions returns the conventional defaults used
// by the rule-based-dimension CSV workflow.
func DefaultRuleBasedDimensionCSVOptions() RuleBasedDimensionCSVOptions {
	return RuleBasedDimensionCSVOptions{
		SeparatorHeader: "%SEPARATOR%",
		IDTemplate:      "rbd_{{.ColumnSlug}}",
		NameTemplate:    "{{.ColumnHeader}}",
		EffectiveAt:     "1970-01",
		ColumnConfig:    make(map[string]RuleBasedDimensionCSVColumnOverride),
		CaseInsensitive: true,
	}
}

var ruleBasedDimensionSlugPattern = regexp.MustCompile(`[^a-z0-9]+`)

func ruleBasedDimensionSlugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = ruleBasedDimensionSlugPattern.ReplaceAllString(s, "_")
	return strings.Trim(s, "_")
}

func ruleBasedDimensionValidName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_':
		default:
			return false
		}
	}
	return true
}

// parseRuleBasedDimensionCondition parses a rule-dimension cell value,
// supporting NOT(...), DIMENSION_CONTAINS(...), DIMENSION_EQUALS(...), or a
// plain value (equivalent to DIMENSION_EQUALS). caseInsensitive is applied to
// the resulting leaf dimension_equals/dimension_contains condition(s).
func parseRuleBasedDimensionCondition(value, dimension string, caseInsensitive bool) FinopsCustomizationsRuleBasedDimensionCondition {
	value = strings.TrimSpace(value)
	if inner, ok := ruleBasedDimensionUnwrap(value, "NOT("); ok {
		if inner == "" {
			return ruleBasedDimensionEqualsCondition(dimension, value, caseInsensitive)
		}
		expr := parseRuleBasedDimensionConditionWithoutNot(inner, dimension, caseInsensitive)
		return FinopsCustomizationsRuleBasedDimensionCondition{Type: Not, Expression: &expr}
	}
	return parseRuleBasedDimensionConditionWithoutNot(value, dimension, caseInsensitive)
}

func parseRuleBasedDimensionConditionWithoutNot(value, dimension string, caseInsensitive bool) FinopsCustomizationsRuleBasedDimensionCondition {
	if inner, ok := ruleBasedDimensionUnwrap(value, "DIMENSION_CONTAINS("); ok {
		if inner == "" {
			return ruleBasedDimensionEqualsCondition(dimension, value, caseInsensitive)
		}
		condition := FinopsCustomizationsRuleBasedDimensionCondition{
			Type:      DimensionContains,
			Dimension: &dimension,
			Substring: &inner,
		}
		if caseInsensitive {
			condition.CaseInsensitive = &caseInsensitive
		}
		return condition
	}
	if inner, ok := ruleBasedDimensionUnwrap(value, "DIMENSION_EQUALS("); ok {
		if inner == "" {
			return ruleBasedDimensionEqualsCondition(dimension, value, caseInsensitive)
		}
		return ruleBasedDimensionEqualsCondition(dimension, inner, caseInsensitive)
	}
	return ruleBasedDimensionEqualsCondition(dimension, value, caseInsensitive)
}

func ruleBasedDimensionUnwrap(value, prefix string) (string, bool) {
	if !strings.HasPrefix(value, prefix) || !strings.HasSuffix(value, ")") {
		return "", false
	}
	return strings.TrimSpace(value[len(prefix) : len(value)-1]), true
}

func ruleBasedDimensionEqualsCondition(dimension, value string, caseInsensitive bool) FinopsCustomizationsRuleBasedDimensionCondition {
	condition := FinopsCustomizationsRuleBasedDimensionCondition{
		Type:      DimensionEquals,
		Dimension: &dimension,
		Value:     &value,
	}
	if caseInsensitive {
		condition.CaseInsensitive = &caseInsensitive
	}
	return condition
}

// parseRuleBasedDimensionValue parses an output cell value, supporting
// {{dimension_name}} or VALUE_FROM_DIMENSION(dimension_name) to populate the
// dimension from another dimension's runtime value, or a plain string
// (static text).
func parseRuleBasedDimensionValue(value string) FinopsCustomizationsRuleBasedDimensionValueExpression {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "{{") && strings.HasSuffix(value, "}}") {
		if dimension := strings.TrimSpace(value[2 : len(value)-2]); dimension != "" {
			return FinopsCustomizationsRuleBasedDimensionValueExpression{Dimension: &dimension}
		}
		return FinopsCustomizationsRuleBasedDimensionValueExpression{Text: &value}
	}
	if inner, ok := ruleBasedDimensionUnwrap(value, "VALUE_FROM_DIMENSION("); ok && inner != "" {
		return FinopsCustomizationsRuleBasedDimensionValueExpression{Dimension: &inner}
	}
	return FinopsCustomizationsRuleBasedDimensionValueExpression{Text: &value}
}

func renderRuleBasedDimensionTemplate(tmplStr string, data RuleBasedDimensionCSVTemplateData) (string, error) {
	tmpl, err := template.New("rbd").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}
	return buf.String(), nil
}

// GenerateRuleBasedDimensionsFromCSVFile opens path and delegates to
// GenerateRuleBasedDimensionsFromCSV.
func GenerateRuleBasedDimensionsFromCSVFile(path string, opts RuleBasedDimensionCSVOptions) ([]RuleBasedDimensionSpec, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening CSV file: %w", err)
	}
	defer func() { _ = f.Close() }()
	return GenerateRuleBasedDimensionsFromCSV(f, opts)
}

// GenerateRuleBasedDimensionsFromCSV converts a CSV document into one
// RuleBasedDimensionSpec per output column (see RuleBasedDimensionCSVOptions),
// ready to pass to RuleBasedDimensionBulkTool.Invoke. It performs no API
// calls; it is a pure, local transformation.
func GenerateRuleBasedDimensionsFromCSV(r io.Reader, opts RuleBasedDimensionCSVOptions) ([]RuleBasedDimensionSpec, error) {
	if strings.TrimSpace(opts.SeparatorHeader) == "" {
		return nil, fmt.Errorf("rule-based-dimension CSV: separator header is required")
	}

	rows, err := csv.NewReader(r).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("reading CSV: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("rule-based-dimension CSV: file is empty")
	}

	headers := make([]string, len(rows[0]))
	for i, h := range rows[0] {
		h = strings.TrimPrefix(strings.TrimSpace(h), "\uFEFF")
		headers[i] = h
	}

	separatorIdx := -1
	for i, h := range headers {
		if h != opts.SeparatorHeader {
			continue
		}
		if separatorIdx != -1 {
			return nil, fmt.Errorf("rule-based-dimension CSV: duplicate separator header %q found at columns %d and %d", opts.SeparatorHeader, separatorIdx, i)
		}
		separatorIdx = i
	}
	if separatorIdx == -1 {
		return nil, fmt.Errorf("rule-based-dimension CSV: separator header %q not found; available headers: %v", opts.SeparatorHeader, headers)
	}

	var dimensionHeaders, outputHeaders []string
	var dimensionIndices, outputIndices []int
	for i, h := range headers {
		switch {
		case i == separatorIdx:
			continue
		case strings.HasPrefix(h, RuleBasedDimensionCSVDescriptionColumnPrefix):
			continue
		case i < separatorIdx:
			dimensionHeaders = append(dimensionHeaders, h)
			dimensionIndices = append(dimensionIndices, i)
		default:
			outputHeaders = append(outputHeaders, h)
			outputIndices = append(outputIndices, i)
		}
	}
	if len(dimensionHeaders) == 0 {
		return nil, fmt.Errorf("rule-based-dimension CSV: no rule dimension columns found (all headers are after the separator)")
	}
	if len(outputHeaders) == 0 {
		return nil, fmt.Errorf("rule-based-dimension CSV: no output columns found (all headers are before the separator)")
	}
	for _, h := range dimensionHeaders {
		if !ruleBasedDimensionValidName(h) {
			return nil, fmt.Errorf("rule-based-dimension CSV: rule dimension column name %q is invalid; must contain only [a-zA-Z0-9_]", h)
		}
	}
	seen := make(map[string]bool, len(dimensionHeaders)+len(outputHeaders))
	for _, h := range append(append([]string(nil), dimensionHeaders...), outputHeaders...) {
		if seen[h] {
			return nil, fmt.Errorf("rule-based-dimension CSV: duplicate header %q", h)
		}
		seen[h] = true
	}

	rules := make([][]FinopsCustomizationsRuleBasedDimensionRulePayload, len(outputHeaders))
	for _, row := range rows[1:] {
		empty := true
		for _, cell := range row {
			if strings.TrimSpace(cell) != "" {
				empty = false
				break
			}
		}
		if empty {
			continue
		}

		var expressions []FinopsCustomizationsRuleBasedDimensionCondition
		for i, csvIdx := range dimensionIndices {
			if csvIdx >= len(row) {
				continue
			}
			cell := strings.TrimSpace(row[csvIdx])
			if cell == "" {
				continue
			}
			expressions = append(expressions, parseRuleBasedDimensionCondition(cell, dimensionHeaders[i], opts.CaseInsensitive))
		}
		if len(expressions) == 0 {
			continue
		}
		var condition FinopsCustomizationsRuleBasedDimensionCondition
		if len(expressions) == 1 {
			condition = expressions[0]
		} else {
			condition = FinopsCustomizationsRuleBasedDimensionCondition{Type: And, Expressions: &expressions}
		}

		for i, csvIdx := range outputIndices {
			if csvIdx >= len(row) {
				continue
			}
			cell := strings.TrimSpace(row[csvIdx])
			if cell == "" {
				continue
			}
			value := parseRuleBasedDimensionValue(cell)
			condition := condition // per-rule copy; condition is reused across output columns
			rules[i] = append(rules[i], FinopsCustomizationsRuleBasedDimensionRulePayload{
				Condition: &condition,
				Value:     value,
			})
		}
	}

	var specs []RuleBasedDimensionSpec
	usedIDs := make(map[string]bool)
	for i, outputHeader := range outputHeaders {
		override, hasOverride := opts.ColumnConfig[outputHeader]
		if hasOverride && override.Skip {
			continue
		}
		if len(rules[i]) == 0 {
			continue
		}

		data := RuleBasedDimensionCSVTemplateData{
			ColumnHeader: outputHeader,
			ColumnSlug:   ruleBasedDimensionSlugify(outputHeader),
			ColumnIndex:  fmt.Sprintf("%d", i),
		}

		id, name := "", ""
		if hasOverride {
			id, name = override.ID, override.Name
		}
		if id == "" {
			slug := ruleBasedDimensionSlugify(outputHeader)
			if strings.HasPrefix(slug, "rbd_") && ruleBasedDimensionValidName(slug) {
				id = slug
			} else {
				rendered, err := renderRuleBasedDimensionTemplate(opts.IDTemplate, data)
				if err != nil {
					return nil, fmt.Errorf("rendering ID template for column %q: %w", outputHeader, err)
				}
				id = rendered
			}
		}
		if name == "" {
			rendered, err := renderRuleBasedDimensionTemplate(opts.NameTemplate, data)
			if err != nil {
				return nil, fmt.Errorf("rendering name template for column %q: %w", outputHeader, err)
			}
			name = rendered
		}
		if !strings.HasPrefix(id, "rbd_") {
			return nil, fmt.Errorf("rule-based-dimension CSV: id %q for column %q must start with the %q prefix", id, outputHeader, "rbd_")
		}
		if usedIDs[id] {
			return nil, fmt.Errorf("rule-based-dimension CSV: duplicate id %q generated for column %q; use ColumnConfig to assign a unique id", id, outputHeader)
		}
		usedIDs[id] = true

		specs = append(specs, RuleBasedDimensionSpec{
			ID:          id,
			Name:        name,
			EffectiveAt: opts.EffectiveAt,
			Rules:       rules[i],
		})
	}

	if len(specs) == 0 {
		return nil, fmt.Errorf("rule-based-dimension CSV: no dimensions generated (all output columns were empty or skipped)")
	}
	return specs, nil
}
