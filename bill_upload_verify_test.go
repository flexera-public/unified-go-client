package flexera

import (
	"strings"
	"testing"
)

func TestVerifyCBIBillUploadCSVValid(t *testing.T) {
	csvData := `UsageAmount,UsageUnit,Cost,UsageStartTime,InvoiceYearMonth
10,hours,1.23,2024-01-01T00:00:00Z,202401
`
	result, err := VerifyCBIBillUploadCSV(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Valid {
		t.Fatalf("expected valid result, got errors: %+v", result.Errors)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("expected no warnings, got: %+v", result.Warnings)
	}
}

func TestVerifyCBIBillUploadCSVMissingRequiredHeader(t *testing.T) {
	csvData := `UsageAmount,UsageUnit,Cost,UsageStartTime
10,hours,1.23,2024-01-01T00:00:00Z
`
	result, err := VerifyCBIBillUploadCSV(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Valid {
		t.Fatalf("expected invalid result due to missing header")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e.Message, "InvoiceYearMonth") && strings.Contains(e.Message, "missing header") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected missing-header error for InvoiceYearMonth, got: %+v", result.Errors)
	}
}

func TestVerifyCBIBillUploadCSVUnexpectedHeader(t *testing.T) {
	csvData := `UsageAmount,UsageUnit,Cost,UsageStartTime,InvoiceYearMonth,NotAHeader
10,hours,1.23,2024-01-01T00:00:00Z,202401,foo
`
	result, err := VerifyCBIBillUploadCSV(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Valid {
		t.Fatalf("expected invalid result due to unexpected header")
	}
	found := false
	for _, e := range result.Errors {
		if e.Header == "NotAHeader" && strings.Contains(e.Message, "unexpected header") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected unexpected-header error for NotAHeader, got: %+v", result.Errors)
	}
}

func TestVerifyCBIBillUploadCSVMissingRequiredData(t *testing.T) {
	csvData := `UsageAmount,UsageUnit,Cost,UsageStartTime,InvoiceYearMonth
10,hours,,2024-01-01T00:00:00Z,202401
`
	result, err := VerifyCBIBillUploadCSV(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Valid {
		t.Fatalf("expected invalid result due to missing required data")
	}
	if len(result.Errors) != 1 || result.Errors[0].Line != 2 || result.Errors[0].Header != "Cost" {
		t.Fatalf("expected single missing-data error on line 2 for Cost, got: %+v", result.Errors)
	}
}

func TestVerifyCBIBillUploadCSVInvalidUsageStartTime(t *testing.T) {
	csvData := `UsageAmount,UsageUnit,Cost,UsageStartTime,InvoiceYearMonth
10,hours,1.23,not-a-date,202401
`
	result, err := VerifyCBIBillUploadCSV(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Valid {
		t.Fatalf("expected invalid result due to bad UsageStartTime")
	}
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Message, "RFC3339") {
		t.Fatalf("expected RFC3339 format error, got: %+v", result.Errors)
	}
}

func TestVerifyCBIBillUploadCSVInvalidInvoiceYearMonth(t *testing.T) {
	csvData := `UsageAmount,UsageUnit,Cost,UsageStartTime,InvoiceYearMonth
10,hours,1.23,2024-01-01T00:00:00Z,2024-01
`
	result, err := VerifyCBIBillUploadCSV(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Valid {
		t.Fatalf("expected invalid result due to bad InvoiceYearMonth")
	}
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Message, "YYYYMM") {
		t.Fatalf("expected YYYYMM format error, got: %+v", result.Errors)
	}
}

func TestVerifyCBIBillUploadCSVCostWithComma(t *testing.T) {
	csvData := `UsageAmount,UsageUnit,Cost,UsageStartTime,InvoiceYearMonth
10,hours,"1,234.56",2024-01-01T00:00:00Z,202401
`
	result, err := VerifyCBIBillUploadCSV(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Valid {
		t.Fatalf("expected invalid result due to comma in Cost")
	}
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Message, "remove the comma") {
		t.Fatalf("expected comma-in-Cost error, got: %+v", result.Errors)
	}
}

func TestVerifyCBIBillUploadCSVCloudVendorAccountIDScientificNotation(t *testing.T) {
	csvData := `CloudVendorAccountID,UsageAmount,UsageUnit,Cost,UsageStartTime,InvoiceYearMonth
1.23456789E+11,10,hours,1.23,2024-01-01T00:00:00Z,202401
`
	result, err := VerifyCBIBillUploadCSV(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Valid {
		t.Fatalf("expected valid result (scientific notation is a warning, not an error), got: %+v", result.Errors)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0].Message, "scientific notation") {
		t.Fatalf("expected scientific-notation warning, got: %+v", result.Warnings)
	}
}

func TestVerifyCBIBillUploadCSVCloudVendorAccountIDShortInteger(t *testing.T) {
	csvData := `CloudVendorAccountID,UsageAmount,UsageUnit,Cost,UsageStartTime,InvoiceYearMonth
123456789,10,hours,1.23,2024-01-01T00:00:00Z,202401
`
	result, err := VerifyCBIBillUploadCSV(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Valid {
		t.Fatalf("expected valid result, got: %+v", result.Errors)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0].Message, "000123456789") {
		t.Fatalf("expected padded-account-id warning, got: %+v", result.Warnings)
	}
}

func TestVerifyCBIBillUploadCSVCloudVendorAccountIDFullLengthNoWarning(t *testing.T) {
	csvData := `CloudVendorAccountID,UsageAmount,UsageUnit,Cost,UsageStartTime,InvoiceYearMonth
123456789012,10,hours,1.23,2024-01-01T00:00:00Z,202401
`
	result, err := VerifyCBIBillUploadCSV(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Valid || len(result.Warnings) != 0 {
		t.Fatalf("expected no warnings for a full 12-digit account ID, got: %+v / %+v", result.Errors, result.Warnings)
	}
}

func TestVerifyCBIBillUploadCSVByteOrderMark(t *testing.T) {
	csvData := "\uFEFFUsageAmount,UsageUnit,Cost,UsageStartTime,InvoiceYearMonth\n10,hours,1.23,2024-01-01T00:00:00Z,202401\n"
	result, err := VerifyCBIBillUploadCSV(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Valid {
		t.Fatalf("expected BOM-prefixed header to be handled, got errors: %+v", result.Errors)
	}
}

func TestVerifyCBIBillUploadCSVEmptyFile(t *testing.T) {
	if _, err := VerifyCBIBillUploadCSV(strings.NewReader("")); err == nil {
		t.Fatalf("expected error for empty CSV file")
	}
}

func TestVerifyCBIBillUploadCSVMalformedCSV(t *testing.T) {
	csvData := "UsageAmount,UsageUnit,Cost,UsageStartTime,InvoiceYearMonth\n10,hours,1.23,2024-01-01T00:00:00Z\n"
	if _, err := VerifyCBIBillUploadCSV(strings.NewReader(csvData)); err == nil {
		t.Fatalf("expected error for row with wrong field count")
	}
}

func TestVerifyCBIBillUploadCSVFileNotFound(t *testing.T) {
	if _, err := VerifyCBIBillUploadCSVFile("/nonexistent/path/does-not-exist.csv"); err == nil {
		t.Fatalf("expected error for nonexistent file")
	}
}
