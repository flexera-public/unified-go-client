package flexera

import (
	"context"
	"fmt"
	"time"
)

// FnmsPollOptions bounds report polling. Zero values use a 2-second interval and
// a 5-minute timeout. Negative durations are invalid. An earlier context deadline
// takes precedence. HTTP or decoding errors are returned without retrying here.
type FnmsPollOptions struct {
	Interval time.Duration
	Timeout  time.Duration
}

func fnmsPollContext(ctx context.Context, options *FnmsPollOptions) (context.Context, context.CancelFunc, time.Duration, error) {
	interval, timeout := 2*time.Second, 5*time.Minute
	if options != nil {
		if options.Interval < 0 || options.Timeout < 0 {
			return nil, nil, 0, fmt.Errorf("FNMS polling: negative interval or timeout")
		}
		if options.Interval > 0 {
			interval = options.Interval
		}
		if options.Timeout > 0 {
			timeout = options.Timeout
		}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	return ctx, cancel, interval, nil
}
func fnmsPollPause(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// FnmsStartReportExecution starts an async execution and returns its job ID.
// A documented 409 deduplication response reuses the existing job. Params retain
// the generated object-valued FilterParams, SearchText, Limit and ForceNewExecution.
func (c *ClientWithResponses) FnmsStartReportExecution(ctx context.Context, orgID int64, reportID string, params *FnmsReportExecuteAsyncParams, editors ...RequestEditorFn) (string, error) {
	resp, err := c.FnmsReportExecuteAsyncWithResponse(ctx, orgID, reportID, params, editors...)
	if err != nil {
		return "", err
	}
	if resp.StatusCode() == 202 && resp.JSON202 != nil && resp.JSON202.JobId != "" {
		return resp.JSON202.JobId, nil
	}
	if resp.StatusCode() == 409 && resp.JSON409 != nil && resp.JSON409.JobId != "" {
		return resp.JSON409.JobId, nil
	}
	if resp.StatusCode() != 202 && resp.StatusCode() != 409 {
		return "", ResponseError(resp.StatusCode(), resp.Body)
	}
	return "", fmt.Errorf("FNMS execution: missing job ID")
}

// FnmsWaitReportExecution polls immediately and then until completion or failure.
// It returns the last status alongside any error, including cancellation. The
// generated retrieve operation can fetch individual pages after completion.
func (c *ClientWithResponses) FnmsWaitReportExecution(ctx context.Context, orgID int64, reportID, jobID string, options *FnmsPollOptions, editors ...RequestEditorFn) (*FnmsAsyncJobStatusResponse, error) {
	ctx, cancel, interval, err := fnmsPollContext(ctx, options)
	if err != nil {
		return nil, err
	}
	defer cancel()
	var last *FnmsAsyncJobStatusResponse
	for {
		if err := ctx.Err(); err != nil {
			return last, err
		}
		resp, err := c.FnmsReportExecuteAsyncStatusWithResponse(ctx, orgID, reportID, jobID, editors...)
		if err != nil {
			return last, err
		}
		if resp.StatusCode() != 200 {
			return last, ResponseError(resp.StatusCode(), resp.Body)
		}
		if resp.JSON200 == nil {
			return last, fmt.Errorf("FNMS execution: missing status response")
		}
		last = resp.JSON200
		switch last.Status {
		case FnmsAsyncJobStatusResponseStatusCompleted:
			return last, nil
		case FnmsAsyncJobStatusResponseStatusErrored:
			if last.ErrorMessage != nil {
				return last, fmt.Errorf("FNMS execution errored: %s", *last.ErrorMessage)
			}
			return last, fmt.Errorf("FNMS execution errored")
		case FnmsAsyncJobStatusResponseStatusCreated, FnmsAsyncJobStatusResponseStatusInProgress:
		default:
			return last, fmt.Errorf("FNMS execution: unknown status %q", last.Status)
		}
		if err := fnmsPollPause(ctx, interval); err != nil {
			return last, err
		}
	}
}

// FnmsExecuteReport starts (or reuses) a job and waits for it. It does not retrieve
// result pages; use FnmsReportExecuteAsyncRetrieveWithResponse with the returned JobId.
func (c *ClientWithResponses) FnmsExecuteReport(ctx context.Context, orgID int64, reportID string, params *FnmsReportExecuteAsyncParams, options *FnmsPollOptions, editors ...RequestEditorFn) (*FnmsAsyncJobStatusResponse, error) {
	ctx, cancel, _, err := fnmsPollContext(ctx, options)
	if err != nil {
		return nil, err
	}
	defer cancel()
	jobID, err := c.FnmsStartReportExecution(ctx, orgID, reportID, params, editors...)
	if err != nil {
		return nil, err
	}
	return c.FnmsWaitReportExecution(ctx, orgID, reportID, jobID, options, editors...)
}

// FnmsStartReportExport requests an export and returns its export ID. Params
// preserve object-valued FilterParams, SearchText and the format Accept header.
func (c *ClientWithResponses) FnmsStartReportExport(ctx context.Context, orgID int64, reportID string, params *FnmsReportExportReportParams, editors ...RequestEditorFn) (string, error) {
	resp, err := c.FnmsReportExportReportWithResponse(ctx, orgID, reportID, params, editors...)
	if err != nil {
		return "", err
	}
	if resp.StatusCode() != 201 {
		return "", ResponseError(resp.StatusCode(), resp.Body)
	}
	if resp.JSON201 == nil || resp.JSON201.Data == nil || resp.JSON201.Data.ExportId == "" {
		return "", fmt.Errorf("FNMS export: missing export ID")
	}
	return resp.JSON201.Data.ExportId, nil
}

// FnmsWaitReportExport waits for a terminal export status. Completed requires a
// nonempty Path; No data found is a successful terminal result without a file.
// Errored and unknown statuses fail. The last status is returned alongside errors.
func (c *ClientWithResponses) FnmsWaitReportExport(ctx context.Context, orgID int64, exportID string, options *FnmsPollOptions, editors ...RequestEditorFn) (*FnmsExportStatus, error) {
	ctx, cancel, interval, err := fnmsPollContext(ctx, options)
	if err != nil {
		return nil, err
	}
	defer cancel()
	var last *FnmsExportStatus
	for {
		if err := ctx.Err(); err != nil {
			return last, err
		}
		resp, err := c.FnmsReportReportExportStatusWithResponse(ctx, orgID, exportID, editors...)
		if err != nil {
			return last, err
		}
		if resp.StatusCode() != 200 {
			return last, ResponseError(resp.StatusCode(), resp.Body)
		}
		if resp.JSON200 == nil || resp.JSON200.Data == nil {
			return last, fmt.Errorf("FNMS export: missing status response")
		}
		last = resp.JSON200.Data
		switch last.Status {
		case FnmsExportStatusStatusCompleted:
			if last.Path == nil || *last.Path == "" {
				return last, fmt.Errorf("FNMS export: completed response missing path")
			}
			return last, nil
		case FnmsExportStatusStatusNoDataFound:
			return last, nil
		case FnmsExportStatusStatusErrored:
			return last, fmt.Errorf("FNMS export errored")
		case FnmsExportStatusStatusCreated, FnmsExportStatusStatusInProgress:
		default:
			return last, fmt.Errorf("FNMS export: unknown status %q", last.Status)
		}
		if err := fnmsPollPause(ctx, interval); err != nil {
			return last, err
		}
	}
}

// FnmsExportReport requests an export and waits for its terminal status. Download
// a completed result's Path with FnmsDownloadReportExport; No data found has no file.
func (c *ClientWithResponses) FnmsExportReport(ctx context.Context, orgID int64, reportID string, params *FnmsReportExportReportParams, options *FnmsPollOptions, editors ...RequestEditorFn) (*FnmsExportStatus, error) {
	ctx, cancel, _, err := fnmsPollContext(ctx, options)
	if err != nil {
		return nil, err
	}
	defer cancel()
	exportID, err := c.FnmsStartReportExport(ctx, orgID, reportID, params, editors...)
	if err != nil {
		return nil, err
	}
	return c.FnmsWaitReportExport(ctx, orgID, exportID, options, editors...)
}
