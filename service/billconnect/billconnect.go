// Package billconnect provides an ergonomic, cohesive facade over the unified
// client's bill-connect operations, which are otherwise spread across ~55
// generated FinopsOnboardingBillConnect* methods (AWS, Azure CSP/EA/MCA,
// Common Bill Ingestion, Databricks, GCP). It is "fat library" surface any
// consumer (CLI, Terraform provider, web app) can use to manage bill connects
// from a single entry point.
//
// Methods are thin forwarders to the generated client and reuse the generated
// request-body/response types verbatim (no redefinition).
package billconnect

import (
	"context"

	flexera "github.com/flexera-public/unified-go-client"
)

// Service is a cohesive entry point for bill-connect operations over a unified
// client.
type Service struct {
	client *flexera.ClientWithResponses
}

// New wraps a unified client with the bill-connect facade.
func New(client *flexera.ClientWithResponses) *Service {
	return &Service{client: client}
}

// ---- base ----

// List returns all bill connects for the org.
func (s *Service) List(ctx context.Context, orgID int, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectIndexResponse, error) {
	return s.client.FinopsOnboardingBillConnectIndexWithResponse(ctx, orgID, editors...)
}

// Validations validates all bill connects for the org.
func (s *Service) Validations(ctx context.Context, orgID int, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectValidateResponse, error) {
	return s.client.FinopsOnboardingBillConnectValidateWithResponse(ctx, orgID, editors...)
}

// ---- AWS ----

func (s *Service) AWSCreateIAMRole(ctx context.Context, orgID int, body flexera.FinopsOnboardingBillConnectAWSCreateIAMRoleJSONRequestBody, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectAWSCreateIAMRoleResponse, error) {
	return s.client.FinopsOnboardingBillConnectAWSCreateIAMRoleWithResponse(ctx, orgID, body, editors...)
}

func (s *Service) AWSUpdateIAMRole(ctx context.Context, orgID int, id string, body flexera.FinopsOnboardingBillConnectAWSUpdateIAMRoleJSONRequestBody, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectAWSUpdateIAMRoleResponse, error) {
	return s.client.FinopsOnboardingBillConnectAWSUpdateIAMRoleWithResponse(ctx, orgID, id, body, editors...)
}

func (s *Service) AWSCreateIAMUser(ctx context.Context, orgID int, body flexera.FinopsOnboardingBillConnectAWSCreateIAMUserJSONRequestBody, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectAWSCreateIAMUserResponse, error) {
	return s.client.FinopsOnboardingBillConnectAWSCreateIAMUserWithResponse(ctx, orgID, body, editors...)
}

func (s *Service) AWSUpdateIAMUser(ctx context.Context, orgID int, id string, body flexera.FinopsOnboardingBillConnectAWSUpdateIAMUserJSONRequestBody, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectAWSUpdateIAMUserResponse, error) {
	return s.client.FinopsOnboardingBillConnectAWSUpdateIAMUserWithResponse(ctx, orgID, id, body, editors...)
}

func (s *Service) AWSGet(ctx context.Context, orgID int, id string, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectAWSShowResponse, error) {
	return s.client.FinopsOnboardingBillConnectAWSShowWithResponse(ctx, orgID, id, editors...)
}

func (s *Service) AWSDelete(ctx context.Context, orgID int, id string, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectAWSDeleteResponse, error) {
	return s.client.FinopsOnboardingBillConnectAWSDeleteWithResponse(ctx, orgID, id, editors...)
}

func (s *Service) AWSValidate(ctx context.Context, orgID int, id string, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectAWSValidateResponse, error) {
	return s.client.FinopsOnboardingBillConnectAWSValidateWithResponse(ctx, orgID, id, editors...)
}

// ---- Azure CSP ----

func (s *Service) AzureCSPCreate(ctx context.Context, orgID int, body flexera.FinopsOnboardingBillConnectAzureCSPCreateJSONRequestBody, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectAzureCSPCreateResponse, error) {
	return s.client.FinopsOnboardingBillConnectAzureCSPCreateWithResponse(ctx, orgID, body, editors...)
}

func (s *Service) AzureCSPUpdate(ctx context.Context, orgID int, id string, body flexera.FinopsOnboardingBillConnectAzureCSPUpdateJSONRequestBody, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectAzureCSPUpdateResponse, error) {
	return s.client.FinopsOnboardingBillConnectAzureCSPUpdateWithResponse(ctx, orgID, id, body, editors...)
}

func (s *Service) AzureCSPGet(ctx context.Context, orgID int, id string, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectAzureCSPShowResponse, error) {
	return s.client.FinopsOnboardingBillConnectAzureCSPShowWithResponse(ctx, orgID, id, editors...)
}

func (s *Service) AzureCSPDelete(ctx context.Context, orgID int, id string, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectAzureCSPDeleteResponse, error) {
	return s.client.FinopsOnboardingBillConnectAzureCSPDeleteWithResponse(ctx, orgID, id, editors...)
}

func (s *Service) AzureCSPValidate(ctx context.Context, orgID int, id string, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectAzureCSPValidateResponse, error) {
	return s.client.FinopsOnboardingBillConnectAzureCSPValidateWithResponse(ctx, orgID, id, editors...)
}

// ---- Azure EA Management ----

func (s *Service) AzureEAManagementCreate(ctx context.Context, orgID int, body flexera.FinopsOnboardingBillConnectAzureEAManagementCreateJSONRequestBody, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectAzureEAManagementCreateResponse, error) {
	return s.client.FinopsOnboardingBillConnectAzureEAManagementCreateWithResponse(ctx, orgID, body, editors...)
}

func (s *Service) AzureEAManagementUpdate(ctx context.Context, orgID int, id string, body flexera.FinopsOnboardingBillConnectAzureEAManagementUpdateJSONRequestBody, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectAzureEAManagementUpdateResponse, error) {
	return s.client.FinopsOnboardingBillConnectAzureEAManagementUpdateWithResponse(ctx, orgID, id, body, editors...)
}

func (s *Service) AzureEAManagementGet(ctx context.Context, orgID int, id string, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectAzureEAManagementShowResponse, error) {
	return s.client.FinopsOnboardingBillConnectAzureEAManagementShowWithResponse(ctx, orgID, id, editors...)
}

func (s *Service) AzureEAManagementDelete(ctx context.Context, orgID int, id string, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectAzureEAManagementDeleteResponse, error) {
	return s.client.FinopsOnboardingBillConnectAzureEAManagementDeleteWithResponse(ctx, orgID, id, editors...)
}

func (s *Service) AzureEAManagementValidate(ctx context.Context, orgID int, id string, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectAzureEAManagementValidateResponse, error) {
	return s.client.FinopsOnboardingBillConnectAzureEAManagementValidateWithResponse(ctx, orgID, id, editors...)
}

// ---- Azure MCA ----

func (s *Service) AzureMCACreate(ctx context.Context, orgID int, body flexera.FinopsOnboardingBillConnectAzureMCACreateMcaBillConnectJSONRequestBody, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectAzureMCACreateMcaBillConnectResponse, error) {
	return s.client.FinopsOnboardingBillConnectAzureMCACreateMcaBillConnectWithResponse(ctx, orgID, body, editors...)
}

func (s *Service) AzureMCAUpdate(ctx context.Context, orgID int, id string, body flexera.FinopsOnboardingBillConnectAzureMCAUpdateMcaBillConnectJSONRequestBody, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectAzureMCAUpdateMcaBillConnectResponse, error) {
	return s.client.FinopsOnboardingBillConnectAzureMCAUpdateMcaBillConnectWithResponse(ctx, orgID, id, body, editors...)
}

func (s *Service) AzureMCAGet(ctx context.Context, orgID int, id string, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectAzureMCAShowResponse, error) {
	return s.client.FinopsOnboardingBillConnectAzureMCAShowWithResponse(ctx, orgID, id, editors...)
}

func (s *Service) AzureMCADelete(ctx context.Context, orgID int, id string, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectAzureMCADeleteResponse, error) {
	return s.client.FinopsOnboardingBillConnectAzureMCADeleteWithResponse(ctx, orgID, id, editors...)
}

func (s *Service) AzureMCAValidate(ctx context.Context, orgID int, id string, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectAzureMCAValidateResponse, error) {
	return s.client.FinopsOnboardingBillConnectAzureMCAValidateWithResponse(ctx, orgID, id, editors...)
}

// ---- Common Bill Ingestion ----

func (s *Service) CommonBillIngestionCreate(ctx context.Context, orgID int, body flexera.FinopsOnboardingBillConnectCommonBillIngestionCreateJSONRequestBody, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectCommonBillIngestionCreateResponse, error) {
	return s.client.FinopsOnboardingBillConnectCommonBillIngestionCreateWithResponse(ctx, orgID, body, editors...)
}

func (s *Service) CommonBillIngestionUpdate(ctx context.Context, orgID int, id string, body flexera.FinopsOnboardingBillConnectCommonBillIngestionUpdateJSONRequestBody, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectCommonBillIngestionUpdateResponse, error) {
	return s.client.FinopsOnboardingBillConnectCommonBillIngestionUpdateWithResponse(ctx, orgID, id, body, editors...)
}

func (s *Service) CommonBillIngestionGet(ctx context.Context, orgID int, id string, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectCommonBillIngestionShowResponse, error) {
	return s.client.FinopsOnboardingBillConnectCommonBillIngestionShowWithResponse(ctx, orgID, id, editors...)
}

func (s *Service) CommonBillIngestionDelete(ctx context.Context, orgID int, id string, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectCommonBillIngestionDeleteResponse, error) {
	return s.client.FinopsOnboardingBillConnectCommonBillIngestionDeleteWithResponse(ctx, orgID, id, editors...)
}

func (s *Service) CommonBillIngestionValidate(ctx context.Context, orgID int, id string, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectCommonBillIngestionValidateResponse, error) {
	return s.client.FinopsOnboardingBillConnectCommonBillIngestionValidateWithResponse(ctx, orgID, id, editors...)
}

// ---- Databricks ----

func (s *Service) DatabricksCreate(ctx context.Context, orgID int, body flexera.FinopsOnboardingBillConnectDatabricksCreateJSONRequestBody, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectDatabricksCreateResponse, error) {
	return s.client.FinopsOnboardingBillConnectDatabricksCreateWithResponse(ctx, orgID, body, editors...)
}

func (s *Service) DatabricksUpdate(ctx context.Context, orgID int, id string, body flexera.FinopsOnboardingBillConnectDatabricksUpdateJSONRequestBody, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectDatabricksUpdateResponse, error) {
	return s.client.FinopsOnboardingBillConnectDatabricksUpdateWithResponse(ctx, orgID, id, body, editors...)
}

func (s *Service) DatabricksGet(ctx context.Context, orgID int, id string, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectDatabricksShowResponse, error) {
	return s.client.FinopsOnboardingBillConnectDatabricksShowWithResponse(ctx, orgID, id, editors...)
}

func (s *Service) DatabricksDelete(ctx context.Context, orgID int, id string, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectDatabricksDeleteResponse, error) {
	return s.client.FinopsOnboardingBillConnectDatabricksDeleteWithResponse(ctx, orgID, id, editors...)
}

func (s *Service) DatabricksValidate(ctx context.Context, orgID int, id string, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectDatabricksValidateResponse, error) {
	return s.client.FinopsOnboardingBillConnectDatabricksValidateWithResponse(ctx, orgID, id, editors...)
}

// ---- GCP ----

func (s *Service) GCPCreate(ctx context.Context, orgID int, body flexera.FinopsOnboardingBillConnectGCPCreateJSONRequestBody, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectGCPCreateResponse, error) {
	return s.client.FinopsOnboardingBillConnectGCPCreateWithResponse(ctx, orgID, body, editors...)
}

func (s *Service) GCPUpdate(ctx context.Context, orgID int, id string, body flexera.FinopsOnboardingBillConnectGCPUpdateJSONRequestBody, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectGCPUpdateResponse, error) {
	return s.client.FinopsOnboardingBillConnectGCPUpdateWithResponse(ctx, orgID, id, body, editors...)
}

func (s *Service) GCPGet(ctx context.Context, orgID int, id string, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectGCPShowResponse, error) {
	return s.client.FinopsOnboardingBillConnectGCPShowWithResponse(ctx, orgID, id, editors...)
}

func (s *Service) GCPDelete(ctx context.Context, orgID int, id string, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectGCPDeleteResponse, error) {
	return s.client.FinopsOnboardingBillConnectGCPDeleteWithResponse(ctx, orgID, id, editors...)
}

func (s *Service) GCPValidate(ctx context.Context, orgID int, id string, editors ...flexera.RequestEditorFn) (*flexera.FinopsOnboardingBillConnectGCPValidateResponse, error) {
	return s.client.FinopsOnboardingBillConnectGCPValidateWithResponse(ctx, orgID, id, editors...)
}
