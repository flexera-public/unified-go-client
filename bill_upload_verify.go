package flexera

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// byteOrderMark is the UTF-8 encoded byte order mark that some spreadsheet
// tools prepend to the first CSV header when saving as "CSV UTF-8".
const byteOrderMark = "\uFEFF"

// CBIBillUploadRequiredHeaders are the CSV headers that must be present, and
// must have a non-empty value on every data row, for a CBI ("cbi-oi-optima")
// bill-upload file.
var CBIBillUploadRequiredHeaders = []string{
	"UsageAmount",
	"UsageUnit",
	"Cost",
	"UsageStartTime",
	"InvoiceYearMonth",
}

// CBIBillUploadValidHeaders are the complete set of CSV headers accepted in a
// CBI bill-upload file. Any header outside this set is reported as an error.
var CBIBillUploadValidHeaders = []string{
	"CloudVendorAccountID",
	"CloudVendorAccountName",
	"Category",
	"InstanceType",
	"LineItemType",
	"Region",
	"ResourceGroup",
	"ResourceType",
	"ResourceID",
	"Service",
	"UsageType",
	"Tags",
	"UsageAmount",
	"UsageUnit",
	"Cost",
	"CurrencyCode",
	"UsageStartTime",
	"InvoiceYearMonth",
	"InvoiceID",
}

// BillUploadVerifyIssue is a single warning or error found while verifying a
// bill-upload CSV file. Line is 1-based and refers to the data row (the
// header row is not counted), matching the alpha CLI's line numbering.
// Line is 0 for issues that apply to the file as a whole (for example a
// missing or unexpected header).
type BillUploadVerifyIssue struct {
	Line    int    `json:"line,omitempty"`
	Header  string `json:"header,omitempty"`
	Message string `json:"message"`
}

func (i BillUploadVerifyIssue) String() string {
	if i.Line > 0 {
		return fmt.Sprintf("line %d: %s", i.Line, i.Message)
	}
	return i.Message
}

// BillUploadVerifyResult reports the outcome of verifying a bill-upload CSV
// file's headers and data.
type BillUploadVerifyResult struct {
	Valid    bool                    `json:"valid"`
	Warnings []BillUploadVerifyIssue `json:"warnings,omitempty"`
	Errors   []BillUploadVerifyIssue `json:"errors,omitempty"`
}

// VerifyCBIBillUploadCSVFile opens the file at path and verifies it as a CBI
// ("cbi-oi-optima") bill-upload CSV file. It is a convenience wrapper around
// VerifyCBIBillUploadCSV.
func VerifyCBIBillUploadCSVFile(path string) (BillUploadVerifyResult, error) {
	file, err := os.Open(path)
	if err != nil {
		return BillUploadVerifyResult{}, fmt.Errorf("bill-upload verify: open %q: %w", path, err)
	}
	defer file.Close()

	return VerifyCBIBillUploadCSV(file)
}

// VerifyCBIBillUploadCSV verifies a CBI ("cbi-oi-optima") bill-upload CSV
// file read from r. It checks that all required headers are present, that no
// unexpected headers are present, that every required field has a value, and
// that several commonly-misformatted fields (UsageStartTime,
// InvoiceYearMonth, CloudVendorAccountID, Cost) are well-formed. It never
// makes network calls; this is purely a local, offline check.
//
// A non-nil error is only returned for structural problems reading the CSV
// itself (for example malformed quoting or inconsistent field counts).
// Content problems are reported in the returned BillUploadVerifyResult as
// warnings or errors, matching the alpha CLI's "verify" behavior.
func VerifyCBIBillUploadCSV(r io.Reader) (BillUploadVerifyResult, error) {
	rows, err := csv.NewReader(r).ReadAll()
	if err != nil {
		return BillUploadVerifyResult{}, fmt.Errorf("bill-upload verify: reading CSV: %w", err)
	}
	if len(rows) == 0 {
		return BillUploadVerifyResult{}, fmt.Errorf("bill-upload verify: CSV file is empty")
	}

	headers := make([]string, len(rows[0]))
	for i, h := range rows[0] {
		headers[i] = strings.TrimPrefix(h, byteOrderMark)
	}
	data := rows[1:]

	result := BillUploadVerifyResult{}
	result.Errors = append(result.Errors, verifyBillUploadHeaders(headers)...)
	warnings, errs := verifyBillUploadData(data, headers)
	result.Warnings = append(result.Warnings, warnings...)
	result.Errors = append(result.Errors, errs...)
	result.Valid = len(result.Errors) == 0

	return result, nil
}

func verifyBillUploadHeaders(headers []string) []BillUploadVerifyIssue {
	var issues []BillUploadVerifyIssue

	present := make(map[string]struct{}, len(headers))
	for _, h := range headers {
		present[h] = struct{}{}
	}
	for _, required := range CBIBillUploadRequiredHeaders {
		if _, ok := present[required]; !ok {
			issues = append(issues, BillUploadVerifyIssue{
				Header:  required,
				Message: fmt.Sprintf("missing header: %q is a required CSV header and was not found", required),
			})
		}
	}

	valid := make(map[string]struct{}, len(CBIBillUploadValidHeaders))
	for _, h := range CBIBillUploadValidHeaders {
		valid[h] = struct{}{}
	}
	for _, h := range headers {
		if _, ok := valid[h]; !ok {
			issues = append(issues, BillUploadVerifyIssue{
				Header:  h,
				Message: fmt.Sprintf("unexpected header: %q is not an expected CSV header", url.QueryEscape(h)),
			})
		}
	}

	return issues
}

func verifyBillUploadData(rows [][]string, headers []string) (warnings, errs []BillUploadVerifyIssue) {
	requiredSet := make(map[string]struct{}, len(CBIBillUploadRequiredHeaders))
	for _, h := range CBIBillUploadRequiredHeaders {
		requiredSet[h] = struct{}{}
	}

	for rowIndex, row := range rows {
		// Data starts on file line 2 (line 1 is the header row).
		line := rowIndex + 2
		for col, field := range row {
			if col >= len(headers) {
				continue
			}
			header := headers[col]

			if field == "" {
				if _, required := requiredSet[header]; required {
					errs = append(errs, BillUploadVerifyIssue{
						Line: line, Header: header,
						Message: fmt.Sprintf("missing data: field %q is empty and required", header),
					})
				}
				continue
			}

			switch header {
			case "UsageStartTime":
				if _, err := time.Parse(time.RFC3339, field); err != nil {
					errs = append(errs, BillUploadVerifyIssue{
						Line: line, Header: header,
						Message: fmt.Sprintf("invalid UsageStartTime: %q is not a valid RFC3339 date format", field),
					})
				}
			case "InvoiceYearMonth":
				if _, err := time.Parse("200601", field); err != nil {
					errs = append(errs, BillUploadVerifyIssue{
						Line: line, Header: header,
						Message: fmt.Sprintf("invalid InvoiceYearMonth: %q is not a valid YYYYMM format", field),
					})
				}
			case "CloudVendorAccountID":
				warnings = append(warnings, verifyBillUploadCloudVendorAccountID(line, field)...)
			case "Cost":
				if strings.Contains(field, ",") {
					errs = append(errs, BillUploadVerifyIssue{
						Line: line, Header: header,
						Message: fmt.Sprintf("invalid Cost: %q is not a valid number format -- please remove the comma", field),
					})
				}
			}
		}
	}

	return warnings, errs
}

func verifyBillUploadCloudVendorAccountID(line int, field string) []BillUploadVerifyIssue {
	var warnings []BillUploadVerifyIssue

	// Excel often converts large numbers (e.g. a 12-digit AWS account ID)
	// into scientific notation when a CSV is opened and re-saved.
	if strings.Contains(field, "E+") {
		warnings = append(warnings, BillUploadVerifyIssue{
			Line: line, Header: "CloudVendorAccountID",
			Message: fmt.Sprintf("%q is in scientific notation; a 12-digit AWS account ID was likely intended", field),
		})
		return warnings
	}

	// Excel also drops leading zeros from numeric-looking cells, so a
	// 12-digit AWS account ID beginning with zeros can shrink below 12
	// digits.
	if n, err := strconv.Atoi(field); err == nil && len(field) < 12 {
		padded := fmt.Sprintf("%012d", n)
		warnings = append(warnings, BillUploadVerifyIssue{
			Line: line, Header: "CloudVendorAccountID",
			Message: fmt.Sprintf("%q is an integer but not 12 digits; a 12-digit AWS account ID (%s) was likely intended", field, padded),
		})
	}

	return warnings
}
