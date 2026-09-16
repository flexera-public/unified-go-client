package flexera

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
)

// MaxBillUploadFiles is the maximum number of files accepted by a bill upload.
const MaxBillUploadFiles = 500

// BillUploadPushClient is the subset of the generated client used by
// BillUploadPushTool.
type BillUploadPushClient interface {
	BillUploadBillUploadCreate(
		ctx context.Context,
		orgID int,
		body BillUploadBillUploadCreateJSONRequestBody,
		reqEditors ...RequestEditorFn,
	) (*http.Response, error)
	BillUploadBillUploadCreateFileWithBody(
		ctx context.Context,
		orgID int,
		billUploadID openapi_types.UUID,
		fileID string,
		contentType string,
		body io.Reader,
		reqEditors ...RequestEditorFn,
	) (*http.Response, error)
	BillUploadBillUploadCreateOperation(
		ctx context.Context,
		orgID int,
		billUploadID openapi_types.UUID,
		body BillUploadBillUploadCreateOperationJSONRequestBody,
		reqEditors ...RequestEditorFn,
	) (*http.Response, error)
}

// BillUploadPushInput describes a complete bill-upload submission.
type BillUploadPushInput struct {
	OrgID         int      `json:"orgId"`
	BillConnectID string   `json:"billConnectId"`
	BillingPeriod string   `json:"billingPeriod"`
	Files         []string `json:"files"`
}

// BillUploadPushOutput identifies the committed bill upload and uploaded files.
type BillUploadPushOutput struct {
	BillUploadID string   `json:"billUploadId"`
	FileIDs      []string `json:"fileIds"`
}

// BillUploadPushTool creates a bill upload, uploads local files, then commits
// it. It never commits when creation or any file upload fails.
type BillUploadPushTool struct {
	client BillUploadPushClient
}

// NewBillUploadPushTool constructs the bill-upload push workflow.
func NewBillUploadPushTool(client BillUploadPushClient) *BillUploadPushTool {
	return &BillUploadPushTool{client: client}
}

func (*BillUploadPushTool) Name() string {
	return "bill-upload push"
}

func (*BillUploadPushTool) Description() string {
	return "Create a bill upload, upload local cost files, and commit it"
}

// Invoke executes the create, upload, and commit sequence.
func (t *BillUploadPushTool) Invoke(ctx context.Context, in BillUploadPushInput) (BillUploadPushOutput, error) {
	if t == nil || t.client == nil {
		return BillUploadPushOutput{}, fmt.Errorf("bill-upload push: client is required")
	}

	in.BillConnectID = strings.TrimSpace(in.BillConnectID)
	in.BillingPeriod = strings.TrimSpace(in.BillingPeriod)
	if err := validateBillUploadPushInput(in); err != nil {
		return BillUploadPushOutput{}, err
	}
	fileIDs, err := validateBillUploadFiles(in.Files)
	if err != nil {
		return BillUploadPushOutput{}, err
	}

	createResponse, err := t.client.BillUploadBillUploadCreate(ctx, in.OrgID, BillUploadBillUploadCreateJSONRequestBody{
		BillConnectId: in.BillConnectID,
		BillingPeriod: in.BillingPeriod,
	})
	if err != nil {
		return BillUploadPushOutput{}, fmt.Errorf("bill-upload push: create: %w", err)
	}
	createBody, err := billUploadPushResponse(createResponse, "create")
	if err != nil {
		return BillUploadPushOutput{}, err
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(createBody, &created); err != nil {
		return BillUploadPushOutput{}, fmt.Errorf("bill-upload push: decode create response: %w", err)
	}
	var billUploadID openapi_types.UUID
	if err := billUploadID.UnmarshalText([]byte(created.ID)); err != nil {
		return BillUploadPushOutput{}, fmt.Errorf("bill-upload push: create response has invalid bill upload ID %q: %w", created.ID, err)
	}

	for i, path := range in.Files {
		file, err := os.Open(path)
		if err != nil {
			return BillUploadPushOutput{}, fmt.Errorf("bill-upload push: open file %q: %w", path, err)
		}
		response, uploadErr := t.client.BillUploadBillUploadCreateFileWithBody(
			ctx, in.OrgID, billUploadID, fileIDs[i], "application/octet-stream", file,
		)
		closeErr := file.Close()
		if uploadErr != nil {
			return BillUploadPushOutput{}, fmt.Errorf("bill-upload push: upload file %q: %w", path, uploadErr)
		}
		if closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
			return BillUploadPushOutput{}, fmt.Errorf("bill-upload push: close file %q: %w", path, closeErr)
		}
		if _, err := billUploadPushResponse(response, fmt.Sprintf("upload file %q", fileIDs[i])); err != nil {
			return BillUploadPushOutput{}, err
		}
	}

	commitResponse, err := t.client.BillUploadBillUploadCreateOperation(ctx, in.OrgID, billUploadID, BillUploadBillUploadCreateOperationJSONRequestBody{
		Operation: Commit,
	})
	if err != nil {
		return BillUploadPushOutput{}, fmt.Errorf("bill-upload push: commit: %w", err)
	}
	if _, err := billUploadPushResponse(commitResponse, "commit"); err != nil {
		return BillUploadPushOutput{}, err
	}

	return BillUploadPushOutput{BillUploadID: billUploadID.String(), FileIDs: fileIDs}, nil
}

func validateBillUploadPushInput(in BillUploadPushInput) error {
	if in.OrgID <= 0 {
		return fmt.Errorf("bill-upload push: orgId must be positive")
	}
	if in.BillConnectID == "" {
		return fmt.Errorf("bill-upload push: billConnectId is required")
	}
	if len(in.BillingPeriod) != len("2006-01") {
		return fmt.Errorf("bill-upload push: billingPeriod must be formatted as yyyy-mm")
	}
	if _, err := time.Parse("2006-01", in.BillingPeriod); err != nil {
		return fmt.Errorf("bill-upload push: billingPeriod must be formatted as yyyy-mm: %w", err)
	}
	return nil
}

func validateBillUploadFiles(paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("bill-upload push: at least one file is required")
	}
	if len(paths) > MaxBillUploadFiles {
		return nil, fmt.Errorf("bill-upload push: at most %d files are allowed", MaxBillUploadFiles)
	}

	fileIDs := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("bill-upload push: stat file %q: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("bill-upload push: file %q is not a regular file", path)
		}
		fileID := filepath.Base(path)
		if len(fileID) > 100 {
			return nil, fmt.Errorf("bill-upload push: file name %q exceeds 100 characters", fileID)
		}
		if _, exists := seen[fileID]; exists {
			return nil, fmt.Errorf("bill-upload push: duplicate file name %q", fileID)
		}
		seen[fileID] = struct{}{}
		fileIDs = append(fileIDs, fileID)
	}
	return fileIDs, nil
}

func billUploadPushResponse(response *http.Response, action string) ([]byte, error) {
	if response == nil {
		return nil, fmt.Errorf("bill-upload push: %s returned no response", action)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("bill-upload push: read %s response: %w", action, err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("bill-upload push: %s returned status %d: %s", action, response.StatusCode, string(body))
	}
	return body, nil
}
