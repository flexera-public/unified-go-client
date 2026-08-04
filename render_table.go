package flexera

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"
)

func WriteTable(w io.Writer, value any) error {
	switch v := value.(type) {
	case *BudgetBudgetList:
		return writeBudgetBudgetListTable(w, v)
	case *BudgetCloudVendorAccountsList:
		return writeBudgetCloudVendorAccountListTable(w, v)
	case *CredOrganizationCredentialsList:
		return writeCredentialListTable(w, v)
	case *CredProjectCredentialsList:
		return writeProjectCredentialListTable(w, v)
	case *FinopsOnboardingGenericBillConnectItemList:
		return writeBillConnectListTable(w, v)
	case *FinopsCustomizationsRuleBasedDimensionList:
		return writeDimensionListTable(w, v)
	case *FinopsCustomizationsTagDimensionList:
		return writeTagDimensionListTable(w, v)
	case *IamUserList:
		return writeUserListTable(w, v)
	case *IamUserOrgList:
		return writeUserOrgListTable(w, v)
	case *IamRoleCollection:
		return writeRoleListTable(w, v)
	case *IamGroupList:
		return writeGroupListTable(w, v)
	case *IamMSPCustomerCollection:
		return writeMSPCustomerListTable(w, v)
	case *IamGroupMembershipCollection:
		return writeGroupMembershipListTable(w, v)
	case *IamServiceAccountCollection:
		return writeServiceAccountListTable(w, v)
	case *IamCapabilityCollection:
		return writeCapabilityListTable(w, v)
	case *[]IamContract:
		return writeContractListTable(w, v)
	case *IamIdentityProviderCollection:
		return writeIdentityProviderListTable(w, v)
	case *IamDomainCollection:
		return writeDomainListTable(w, v)
	case *IamSAML2IdentityProviderSigningKeyCollection:
		return writeSigningKeyListTable(w, v)
	case *IamSCIMGroupList:
		return writeSCIMGroupListTable(w, v)
	case *IamSCIMUserList:
		return writeSCIMUserListTable(w, v)
	case *IamInvitationCollection:
		return writeOrganizationInvitationListTable(w, v)
	case *IamAccessRuleList:
		return writeAccessRuleListTable(w, v)
	case *IamAPIEventCollection:
		return writeAPIEventListTable(w, v)
	case *IamEventCollection:
		return writeEventListTable(w, v)
	case *IamCustomizationTypeList:
		return writeCustomizationTypeListTable(w, v)
	case *IamCustomizationList:
		return writeCustomizationListTable(w, v)
	case *IamServiceAccountClientCollection:
		return writeServiceAccountClientListTable(w, v)
	case *[]IamFlexeraIamRefreshToken:
		return writeRefreshTokenListTable(w, v)
	case *IamIPAccessControlRuleList:
		return writeIPAccessControlRuleListTable(w, v)
	case *PolicyPublishedTemplateList:
		return writePolicyPublishedTemplateListTable(w, v)
	case *PolicyCustomCatalogTemplateList:
		return writePolicyCustomCatalogListTable(w, v)
	case *PolicyCustomizationTypesList:
		return writePolicyCustomizationTypeListTable(w, v)
	case *PolicyCustomizationValuesList:
		return writePolicyCustomizationValueListTable(w, v)
	case *PolicyPolicyManagerList:
		return writePolicyManagerListTable(w, v)
	case *PolicyIncidentAggregateList:
		return writePolicyIncidentAggregateListTable(w, v)
	case *PolicyPolicyAggregateList:
		return writePolicyAggregateListTable(w, v)
	case *PolicyAppliedPolicyList:
		return writePolicyAppliedPolicyListTable(w, v)
	case *PolicyActionStatusList:
		return writePolicyActionStatusListTable(w, v)
	case *PolicyArchivedIncidentList:
		return writePolicyArchivedIncidentListTable(w, v)
	case *PolicyPolicyTemplateList:
		return writePolicyTemplateListTable(w, v)
	case *PolicyIncidentAggregateUnmanaged:
		return writePolicyUnmanagedIncidentListTable(w, v)
	case *PolicyPolicyAggregateUnmanaged:
		return writePolicyUnmanagedAppliedPolicyListTable(w, v)
	case map[string]any:
		return writeMapTable(w, v)
	default:
		return fmt.Errorf("table output is only supported for list commands, not %T", value)
	}
}

// writeMapTable handles paginated responses from pagination.Collect, which always returns
// map[string]any. It dispatches to the appropriate typed table writer based on the "kind" field.
func writeMapTable(w io.Writer, m map[string]any) error {
	b, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("table output is only supported for list commands, not map[string]interface {}")
	}
	kind, _ := m["kind"].(string)
	switch kind {
	case "iam:api-event-collection":
		var typed IamAPIEventCollection
		if err := json.Unmarshal(b, &typed); err != nil {
			return err
		}
		return writeAPIEventListTable(w, &typed)
	case "iam:event-collection":
		var typed IamEventCollection
		if err := json.Unmarshal(b, &typed); err != nil {
			return err
		}
		return writeEventListTable(w, &typed)
	case "iam:customization-type-list":
		var typed IamCustomizationTypeList
		if err := json.Unmarshal(b, &typed); err != nil {
			return err
		}
		return writeCustomizationTypeListTable(w, &typed)
	case "iam:customization-list":
		var typed IamCustomizationList
		if err := json.Unmarshal(b, &typed); err != nil {
			return err
		}
		return writeCustomizationListTable(w, &typed)
	case "policy:published-template-list":
		var typed PolicyPublishedTemplateList
		if err := json.Unmarshal(b, &typed); err != nil {
			return err
		}
		return writePolicyPublishedTemplateListTable(w, &typed)
	case "policy:custom-catalog-template-list":
		var typed PolicyCustomCatalogTemplateList
		if err := json.Unmarshal(b, &typed); err != nil {
			return err
		}
		return writePolicyCustomCatalogListTable(w, &typed)
	case "policy:incident-aggregate-list":
		var typed PolicyIncidentAggregateList
		if err := json.Unmarshal(b, &typed); err != nil {
			return err
		}
		return writePolicyIncidentAggregateListTable(w, &typed)
	case "policy:policy-aggregate-list":
		var typed PolicyPolicyAggregateList
		if err := json.Unmarshal(b, &typed); err != nil {
			return err
		}
		return writePolicyAggregateListTable(w, &typed)
	default:
		return fmt.Errorf("table output is only supported for list commands, not map[string]interface {}")
	}
}

func writeBudgetBudgetListTable(w io.Writer, list *BudgetBudgetList) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tMETRIC\tDIMENSIONS\tUPDATED_AT")
	for _, item := range list.Values {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			item.Id,
			item.Name,
			item.Metric,
			strings.Join(item.Dimensions, ","),
			formatTime(item.UpdatedAt),
		)
	}
	return tw.Flush()
}

func writeBudgetCloudVendorAccountListTable(w io.Writer, list *BudgetCloudVendorAccountsList) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "NAME\tKIND\tTAGS\tLAST_DISCOVERED_AT")
	for _, item := range list.Values {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%s\n",
			item.Name,
			item.Kind,
			len(item.Tags),
			formatTime(item.LastDiscoveredAt),
		)
	}
	return tw.Flush()
}

func writeCredentialListTable(w io.Writer, list *CredOrganizationCredentialsList) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tSCHEME\tORG_ID\tUPDATED_AT")
	if list.Values != nil {
		for _, item := range *list.Values {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n",
				item.Id,
				item.Name,
				item.Scheme,
				item.OrgId,
				formatTime(item.UpdatedAt),
			)
		}
	}
	return tw.Flush()
}

func writeProjectCredentialListTable(w io.Writer, list *CredProjectCredentialsList) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tSCHEME\tPROJECT_ID\tUPDATED_AT")
	if list.Values != nil {
		for _, item := range *list.Values {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n",
				item.Id,
				item.Name,
				item.Scheme,
				item.ProjectId,
				formatTime(item.UpdatedAt),
			)
		}
	}
	return tw.Flush()
}

func writeBillConnectListTable(w io.Writer, list *FinopsOnboardingGenericBillConnectItemList) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tKIND\tPROVIDER\tUPDATED_AT")
	if list.Values != nil {
		for _, item := range *list.Values {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
				item.Id,
				item.Kind,
				billConnectProvider(item),
				formatTime(item.UpdatedAt),
			)
		}
	}
	return tw.Flush()
}

func writeDimensionListTable(w io.Writer, list *FinopsCustomizationsRuleBasedDimensionList) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tRULE_LISTS\tUPDATED_AT")
	for _, item := range list.Values {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%s\n",
			item.Id,
			item.Name,
			len(item.RuleListLinks),
			formatTime(item.UpdatedAt),
		)
	}
	return tw.Flush()
}

func writeTagDimensionListTable(w io.Writer, list *FinopsCustomizationsTagDimensionList) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tTAGS\tUPDATED_AT")
	for _, item := range list.Values {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			item.Id,
			item.Name,
			tagDimensionKeys(item.Tags),
			formatTime(item.UpdatedAt),
		)
	}
	return tw.Flush()
}

func writeUserListTable(w io.Writer, list *IamUserList) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tEMAIL\tSTATUS\tACCESS_SOURCE\tLAST_UI_LOGIN\tLAST_API_LOGIN")
	for _, item := range list.Values {
		_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\t%s\n",
			item.Id,
			userDisplayName(item),
			item.Email,
			stringPointerValue((*string)(item.Status)),
			stringPointerValue((*string)(item.AccessSourceDisplayName)),
			formatTimePtr(item.LastUILogin),
			formatTimePtr(item.LastAPILogin),
		)
	}
	return tw.Flush()
}

func writeUserOrgListTable(w io.Writer, list *IamUserOrgList) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tREF")
	for _, item := range list.Values {
		_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\n",
			item.Id,
			item.Name,
			stringValue(item.Ref),
		)
	}
	return tw.Flush()
}

func writeRoleListTable(w io.Writer, list *IamRoleCollection) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tDISPLAY_NAME\tNAME\tCATEGORY\tCAPABILITY\tPRIVILEGES")
	for _, item := range *list {
		_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%d\n",
			item.Id,
			stringPointerValue(item.DisplayName),
			stringPointerValue(item.Name),
			stringPointerValue(item.Category),
			stringPointerValue(item.Capability),
			stringSlicePointerLen(item.Privileges),
		)
	}
	return tw.Flush()
}

func writeGroupListTable(w io.Writer, list *IamGroupList) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tDESCRIPTION\tUPDATED_AT")
	for _, item := range list.Values {
		_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n",
			item.Id,
			item.Name,
			stringPointerValue(item.Description),
			formatTimePtr(item.UpdatedAt),
		)
	}
	return tw.Flush()
}

func writeMSPCustomerListTable(w io.Writer, list *IamMSPCustomerCollection) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tEXTERNAL_ID\tOWNERS\tUPDATED_AT")
	for _, item := range *list {
		_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%d\t%s\n",
			item.Id,
			item.Name,
			stringPointerValue(item.ExternalId),
			scimOwnerCount(item.Owners),
			formatTime(item.UpdatedAt),
		)
	}
	return tw.Flush()
}

func writeGroupMembershipListTable(w io.Writer, list *IamGroupMembershipCollection) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tUSER_ID\tUSER_NAME\tUSER_EMAIL")
	for _, item := range *list {
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%s\t%s\n",
			item.Id,
			groupMembershipUserID(item),
			groupMembershipUserName(item),
			groupMembershipUserEmail(item),
		)
	}
	return tw.Flush()
}

func writeServiceAccountListTable(w io.Writer, list *IamServiceAccountCollection) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tDESCRIPTION\tCREATED_BY\tUPDATED_AT")
	for _, item := range *list {
		_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%d\t%s\n",
			item.Id,
			item.Name,
			stringPointerValue(item.Description),
			item.CreatedBy,
			formatTimePtr(item.UpdatedAt),
		)
	}
	return tw.Flush()
}

func writeCapabilityListTable(w io.Writer, list *IamCapabilityCollection) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "NAME\tSTATUS\tORG_ID\tEXPIRES_AT\tUPDATED_AT")
	for _, item := range *list {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\n",
			item.CapabilityName,
			stringPointerValue(item.OnboardingStatus),
			item.OrgId,
			formatTimePtr(item.ExpiresAt),
			formatTime(item.UpdatedAt),
		)
	}
	return tw.Flush()
}

func writeContractListTable(w io.Writer, list *[]IamContract) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tTITLE\tSTATUS\tINITIATOR_ORG\tTARGET_ORG\tUPDATED_AT")
	for _, item := range *list {
		_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%d\t%d\t%s\n",
			item.Id,
			stringPointerValue(item.Title),
			contractStatus(item),
			item.InitiatorOrgId,
			item.TargetOrgId,
			formatTimePtr(item.UpdatedAt),
		)
	}
	return tw.Flush()
}

func writeIdentityProviderListTable(w io.Writer, list *IamIdentityProviderCollection) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tDISCOVERY_HINT\tISSUER_URI\tUPDATED_AT")
	for _, item := range *list {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			item.Id,
			item.Name,
			stringPointerValue(item.DiscoveryHint),
			stringPointerValue(item.IssuerUri),
			formatTime(item.UpdatedAt),
		)
	}
	return tw.Flush()
}

func writeDomainListTable(w io.Writer, list *IamDomainCollection) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "NAME\tIDENTITY_PROVIDER_ID\tVERIFIED_AT\tCREATED_AT")
	for _, item := range *list {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			item.Name,
			item.IdentityProviderId,
			formatTimePtr(item.VerifiedAt),
			formatTime(item.CreatedAt),
		)
	}
	return tw.Flush()
}

func writeSigningKeyListTable(w io.Writer, list *IamSAML2IdentityProviderSigningKeyCollection) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tIDENTITY_PROVIDER_ID\tCREATED_AT\tEXPIRES_AT")
	for _, item := range *list {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			item.Id,
			item.IdentityProviderId,
			formatTime(item.CreatedAt),
			formatTime(item.ExpiresAt),
		)
	}
	return tw.Flush()
}

func writeSCIMGroupListTable(w io.Writer, list *IamSCIMGroupList) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tDISPLAY_NAME\tMEMBERS\tLAST_MODIFIED")
	if list.Resources != nil {
		for _, item := range *list.Resources {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%s\n",
				item.Id,
				item.DisplayName,
				scimGroupMemberCount(item.Members),
				scimMetaLastModified(item.Meta),
			)
		}
	}
	return tw.Flush()
}

func writeSCIMUserListTable(w io.Writer, list *IamSCIMUserList) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tUSER_NAME\tNAME\tACTIVE\tEMAIL\tLAST_MODIFIED")
	if list.Resources != nil {
		for _, item := range *list.Resources {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
				item.Id,
				item.UserName,
				scimUserDisplayName(item.Name),
				boolPointerValue(item.Active),
				scimUserPrimaryValue(item.Emails),
				scimMetaLastModified(item.Meta),
			)
		}
	}
	return tw.Flush()
}

func writeOrganizationInvitationListTable(w io.Writer, list *IamInvitationCollection) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tINVITEE_EMAIL\tSTATUS\tORG\tINVITED_BY\tEXPIRES_AT")
	for _, item := range *list {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			item.Id,
			item.InviteeEmail,
			string(item.Status),
			invitationOrgName(item.Org),
			invitedByName(item.InvitedBy),
			formatTimePtr(item.ExpiresAt),
		)
	}
	return tw.Flush()
}

func writeAccessRuleListTable(w io.Writer, list *IamAccessRuleList) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "SUBJECT\tROLE\tSCOPE\tCREATED_AT")
	for _, item := range list.Values {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			accessRuleSubjectDisplay(item.Subject),
			item.Role.Name,
			accessRuleScopeDisplay(item.Scope),
			formatTime(item.CreatedAt),
		)
	}
	return tw.Flush()
}

func writeAPIEventListTable(w io.Writer, list *IamAPIEventCollection) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "TIMESTAMP\tMETHOD\tPATH\tSTATUS\tPRINCIPAL")
	for _, item := range list.Values {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n",
			formatTime(item.Timestamp),
			item.Request.HttpMethod,
			item.Request.HttpPath,
			item.Response.HttpCode,
			apiEventPrincipalDisplay(item.Principal),
		)
	}
	return tw.Flush()
}

func writeEventListTable(w io.Writer, list *IamEventCollection) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tTIMESTAMP\tTYPE\tRESULT\tPRINCIPAL")
	for _, item := range list.Values {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			item.Id,
			formatTime(item.Timestamp),
			item.EventType,
			string(item.Outcome.Result),
			eventPrincipalDisplay(item.Principal),
		)
	}
	return tw.Flush()
}

func writeCustomizationTypeListTable(w io.Writer, list *IamCustomizationTypeList) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tVALUE_TYPE\tAUTH_REQUIRED\tDESCRIPTION")
	for _, item := range list.Values {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%t\t%s\n",
			item.Id,
			item.ValueType,
			item.AuthenticationRequired,
			item.Description,
		)
	}
	return tw.Flush()
}

func writeCustomizationListTable(w io.Writer, list *IamCustomizationList) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tVALUE\tUPDATED_BY\tUPDATED_AT")
	for _, item := range list.Values {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			item.Id,
			customizationValue(item),
			customizationPrincipalName(item.LastUpdatedBy),
			formatTime(item.UpdatedAt),
		)
	}
	return tw.Flush()
}

func writeServiceAccountClientListTable(w io.Writer, list *IamServiceAccountClientCollection) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "CLIENT_ID\tCREATED_BY\tCREATED_AT")
	for _, item := range *list {
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%s\n",
			item.ClientId,
			item.CreatedBy,
			formatTime(item.CreatedAt),
		)
	}
	return tw.Flush()
}

func writeRefreshTokenListTable(w io.Writer, list *[]IamFlexeraIamRefreshToken) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tDELETABLE\tCREATED_AT\tEXPIRES_AT")
	for _, item := range *list {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			item.Id,
			boolPointerValue(item.IsDeletable),
			formatTime(item.CreatedAt),
			formatTimePtr(item.ExpiresAt),
		)
	}
	return tw.Flush()
}

func writeIPAccessControlRuleListTable(w io.Writer, list *IamIPAccessControlRuleList) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "MATCH\tIP_TYPE\tEFFECT\tNAME")
	for _, item := range list.Rules {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			item.Match,
			item.IpType,
			item.Effect,
			stringPointerValue(item.Name),
		)
	}
	return tw.Flush()
}

func writePolicyPublishedTemplateListTable(w io.Writer, list *PolicyPublishedTemplateList) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tCATEGORY\tBUILT_IN\tHIDDEN\tUPDATED_AT")
	if list.Values != nil {
		for _, item := range *list.Values {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
				item.Id,
				item.Name,
				stringPointerValue(item.Category),
				boolPointerValue(item.BuiltIn),
				boolPointerValue(item.Hidden),
				formatTimePtr(item.UpdatedAt),
			)
		}
	}
	return tw.Flush()
}

func writePolicyCustomCatalogListTable(w io.Writer, list *PolicyCustomCatalogTemplateList) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tCATEGORY\tBUILT_IN\tVISIBLE_TO_CHILD_ORGS\tUPDATED_AT")
	if list.Values != nil {
		for _, item := range *list.Values {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
				item.Id,
				item.Name,
				stringPointerValue(item.Category),
				boolPointerValue(item.BuiltIn),
				policyCustomCatalogChildVisibility(item.CustomCatalog),
				formatTimePtr(item.UpdatedAt),
			)
		}
	}
	return tw.Flush()
}

func writePolicyCustomizationTypeListTable(w io.Writer, list *PolicyCustomizationTypesList) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tDESCRIPTION\tEXAMPLE")
	for _, item := range list.Values {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			item.Id,
			stringPointerValue(item.Name),
			stringPointerValue(item.Description),
			stringPointerValue(item.Example),
		)
	}
	return tw.Flush()
}

func writePolicyCustomizationValueListTable(w io.Writer, list *PolicyCustomizationValuesList) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "CUSTOMIZATION_TYPE_ID\tVALUE\tCREATED_BY\tUPDATED_BY\tUPDATED_AT")
	for _, item := range list.Values {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			item.CustomizationTypeId,
			item.Value,
			policyPrincipalName(item.CreatedBy),
			policyPrincipalName(item.UpdatedBy),
			formatTime(item.UpdatedAt),
		)
	}
	return tw.Flush()
}

func writePolicyManagerListTable(w io.Writer, list *PolicyPolicyManagerList) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tSTATUS\tTEMPLATE\tACTIVE\tERROR\tUPDATED_AT")
	if list.Values != nil {
		for _, item := range *list.Values {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				item.Id,
				item.Name,
				strings.TrimSpace(string(item.Status)),
				policyPublishedTemplateLinkName(item.PublishedTemplate),
				int64PointerValue(item.ActiveCount),
				int64PointerValue(item.ErrorCount),
				formatTimePtr(item.UpdatedAt),
			)
		}
	}
	return tw.Flush()
}

func writePolicyIncidentAggregateListTable(w io.Writer, list *PolicyIncidentAggregateList) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tCATEGORY\tSEVERITY\tSTATE\tCOUNT\tPOLICY_AGGREGATE\tUPDATED_AT")
	if list.Values != nil {
		for _, item := range *list.Values {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				item.Id,
				stringPointerValue(item.Category),
				policyIncidentSeverity(item.Severity),
				policyIncidentState(item.State),
				int64PointerValue(item.Count),
				policyAggregateLinkName(item.PolicyAggregate),
				formatTimePtr(item.UpdatedAt),
			)
		}
	}
	return tw.Flush()
}

func writePolicyAggregateListTable(w io.Writer, list *PolicyPolicyAggregateList) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tSTATUS\tTEMPLATE\tCOUNT\tACTIVE\tERROR\tUPDATED_AT")
	if list.Values != nil {
		for _, item := range *list.Values {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				item.Id,
				item.Name,
				policyAggregateStatus(item.Status),
				policyPublishedTemplateLinkName(item.PublishedTemplate),
				int64PointerValue(item.Count),
				int64PointerValue(item.ActiveCount),
				int64PointerValue(item.ErrorCount),
				formatTimePtr(item.UpdatedAt),
			)
		}
	}
	return tw.Flush()
}

func writePolicyAppliedPolicyListTable(w io.Writer, list *PolicyAppliedPolicyList) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tSTATUS\tSCOPE\tTEMPLATE\tUPDATED_AT")
	if list.Values != nil {
		for _, item := range *list.Values {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
				item.Id,
				item.Name,
				policyAppliedPolicyStatus(item.Status),
				policyAppliedPolicyScope(item.Scope),
				policyAppliedPolicyTemplateName(item.PolicyTemplate, item.PublishedTemplate),
				formatTimePtr(item.UpdatedAt),
			)
		}
	}
	return tw.Flush()
}

func writePolicyActionStatusListTable(w io.Writer, list *PolicyActionStatusList) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tTYPE\tSTATUS\tAPPLIED_POLICY\tINCIDENT\tSTARTED_AT")
	if list.Values != nil {
		for _, item := range *list.Values {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				item.Id,
				item.Name,
				strings.TrimSpace(string(item.Type)),
				strings.TrimSpace(string(item.Status)),
				policyActionStatusAppliedPolicyID(item.AppliedPolicy),
				policyActionStatusIncidentID(item.Incident),
				formatTimePtr(item.StartedAt),
			)
		}
	}
	return tw.Flush()
}

func writePolicyArchivedIncidentListTable(w io.Writer, list *PolicyArchivedIncidentList) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tSUMMARY\tSTATE\tSEVERITY\tAPPLIED_POLICY\tUPDATED_AT")
	if list.Values != nil {
		for _, item := range *list.Values {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
				item.Id,
				stringPointerValue(item.Summary),
				policyArchivedIncidentState(item.State),
				policyArchivedIncidentSeverity(item.Severity),
				policyArchivedIncidentAppliedPolicyName(item.AppliedPolicy),
				formatTimePtr(item.UpdatedAt),
			)
		}
	}
	return tw.Flush()
}

func writePolicyTemplateListTable(w io.Writer, list *PolicyPolicyTemplateList) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tCATEGORY\tPROJECT_ID\tUPDATED_AT")
	if list.Values != nil {
		for _, item := range *list.Values {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
				item.Id,
				item.Name,
				stringPointerValue(item.Category),
				int64PointerValue(item.ProjectId),
				formatTimePtr(item.UpdatedAt),
			)
		}
	}
	return tw.Flush()
}

func writePolicyUnmanagedIncidentListTable(w io.Writer, list *PolicyIncidentAggregateUnmanaged) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tPROJECT\tAPPLIED_POLICY\tSTATE\tSEVERITY\tUPDATED_AT")
	if list.Values != nil {
		for _, item := range *list.Values {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
				item.Id,
				policyProjectName(item.Project),
				policyAppliedPolicyLinkName(item.AppliedPolicy),
				policyUnmanagedIncidentState(item.State),
				policyUnmanagedIncidentSeverity(item.Severity),
				formatTimePtr(item.UpdatedAt),
			)
		}
	}
	return tw.Flush()
}

func writePolicyUnmanagedAppliedPolicyListTable(w io.Writer, list *PolicyPolicyAggregateUnmanaged) error {
	tw := newTabWriter(w)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tPROJECT\tSTATUS\tTEMPLATE\tUPDATED_AT")
	if list.Values != nil {
		for _, item := range *list.Values {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
				item.Id,
				stringPointerValue(item.Name),
				policyProjectName(item.Project),
				policyUnmanagedAppliedPolicyStatus(item.Status),
				policyTemplateLinkName(item.PolicyTemplate),
				formatTimePtr(item.UpdatedAt),
			)
		}
	}
	return tw.Flush()
}

func newTabWriter(w io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.UTC().Format(time.RFC3339)
}

func formatTimePtr(value *time.Time) string {
	if value == nil {
		return "-"
	}
	return formatTime(*value)
}

func billConnectProvider(item FinopsOnboardingFlexeraFinopsOnboardingGenericBillConnectItem) string {
	switch {
	case item.Aws != nil:
		return "aws"
	case item.AzureCsp != nil:
		return "azure-csp"
	case item.AzureEa != nil:
		return "azure-ea"
	case item.AzureEaManagement != nil:
		return "azure-ea-management"
	case item.AzureMcaEnterprise != nil:
		return "azure-mca-enterprise"
	case item.Cbi != nil:
		return "cbi"
	case item.CbiAzureMca != nil:
		return "cbi-azure-mca"
	case item.Databricks != nil:
		return "databricks"
	case item.Gcp != nil:
		return "gcp"
	default:
		return "unknown"
	}
}

func tagDimensionKeys(tags []FinopsCustomizationsTagDimensionTag) string {
	if len(tags) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(tags))
	for _, tag := range tags {
		keys = append(keys, tag.Key)
	}
	return strings.Join(keys, ",")
}

func userDisplayName(user IamFlexeraIamUser) string {
	parts := make([]string, 0, 2)
	if user.FirstName != nil && strings.TrimSpace(*user.FirstName) != "" {
		parts = append(parts, strings.TrimSpace(*user.FirstName))
	}
	if user.LastName != nil && strings.TrimSpace(*user.LastName) != "" {
		parts = append(parts, strings.TrimSpace(*user.LastName))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, " ")
}

func stringPointerValue(value *string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return "-"
	}
	return strings.TrimSpace(*value)
}

func stringValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return strings.TrimSpace(value)
}

func stringSlicePointerLen(value *[]string) int {
	if value == nil {
		return 0
	}
	return len(*value)
}

func invitationOrgName(org *IamFlexeraIamOrgInvitation) string {
	if org == nil || strings.TrimSpace(org.Name) == "" {
		return "-"
	}
	return strings.TrimSpace(org.Name)
}

func invitedByName(invitedBy IamFlexeraIamInvitedBy) string {
	parts := make([]string, 0, 2)
	if strings.TrimSpace(invitedBy.FirstName) != "" {
		parts = append(parts, strings.TrimSpace(invitedBy.FirstName))
	}
	if strings.TrimSpace(invitedBy.LastName) != "" {
		parts = append(parts, strings.TrimSpace(invitedBy.LastName))
	}
	if len(parts) == 0 {
		return invitedBy.Email
	}
	return strings.Join(parts, " ")
}

func accessRuleSubjectDisplay(subject IamFlexeraIamSubject) string {
	parts := make([]string, 0, 2)
	if subject.FirstName != nil && strings.TrimSpace(*subject.FirstName) != "" {
		parts = append(parts, strings.TrimSpace(*subject.FirstName))
	}
	if subject.LastName != nil && strings.TrimSpace(*subject.LastName) != "" {
		parts = append(parts, strings.TrimSpace(*subject.LastName))
	}
	if len(parts) > 0 {
		return strings.Join(parts, " ")
	}
	if subject.Name != nil && strings.TrimSpace(*subject.Name) != "" {
		return strings.TrimSpace(*subject.Name)
	}
	if subject.Email != nil && strings.TrimSpace(*subject.Email) != "" {
		return strings.TrimSpace(*subject.Email)
	}
	return subject.Ref
}

func accessRuleScopeDisplay(scope IamFlexeraIamScope) string {
	if strings.TrimSpace(scope.Ref) != "" {
		return strings.TrimSpace(scope.Ref)
	}
	return fmt.Sprintf("%s:%d", scope.Kind, scope.Id)
}

func apiEventPrincipalDisplay(principal *IamFlexeraAPIRequestPrincipalMedia) string {
	if principal == nil {
		return "-"
	}
	parts := make([]string, 0, 2)
	if principal.FirstName != nil && strings.TrimSpace(*principal.FirstName) != "" {
		parts = append(parts, strings.TrimSpace(*principal.FirstName))
	}
	if principal.LastName != nil && strings.TrimSpace(*principal.LastName) != "" {
		parts = append(parts, strings.TrimSpace(*principal.LastName))
	}
	if len(parts) > 0 {
		return strings.Join(parts, " ")
	}
	if strings.TrimSpace(principal.Name) != "" {
		return strings.TrimSpace(principal.Name)
	}
	if principal.Email != nil && strings.TrimSpace(*principal.Email) != "" {
		return strings.TrimSpace(*principal.Email)
	}
	return principal.Ref
}

func eventPrincipalDisplay(principal map[string]interface{}) string {
	if principal == nil {
		return "-"
	}
	if name, ok := principal["name"].(string); ok && strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	if email, ok := principal["email"].(string); ok && strings.TrimSpace(email) != "" {
		return strings.TrimSpace(email)
	}
	if ref, ok := principal["ref"].(string); ok && strings.TrimSpace(ref) != "" {
		return strings.TrimSpace(ref)
	}
	return "-"
}

func customizationValue(item IamFlexeraIamCustomization) string {
	if item.StringValue != nil && strings.TrimSpace(*item.StringValue) != "" {
		return strings.TrimSpace(*item.StringValue)
	}
	if item.UrlValue != nil && strings.TrimSpace(*item.UrlValue) != "" {
		return strings.TrimSpace(*item.UrlValue)
	}
	if item.IntegerValue != nil {
		return fmt.Sprintf("%d", *item.IntegerValue)
	}
	if item.BooleanValue != nil {
		return fmt.Sprintf("%t", *item.BooleanValue)
	}
	return "-"
}

func customizationPrincipalName(principal *IamFlexeraIamPrincipal) string {
	if principal == nil {
		return "-"
	}
	if strings.TrimSpace(principal.Name) != "" {
		return strings.TrimSpace(principal.Name)
	}
	return strings.TrimSpace(principal.Ref)
}

func boolPointerValue(value *bool) string {
	if value == nil {
		return "-"
	}
	if *value {
		return "true"
	}
	return "false"
}

func scimOwnerCount(owners *[]IamFlexeraIamUser) int {
	if owners == nil {
		return 0
	}
	return len(*owners)
}

func scimGroupMemberCount(members *[]IamSCIMGroupMember) int {
	if members == nil {
		return 0
	}
	return len(*members)
}

func scimMetaLastModified(meta *IamSCIMResourceMeta) string {
	if meta == nil {
		return "-"
	}
	return formatTime(meta.LastModified)
}

func scimUserDisplayName(name *IamSCIMUserName) string {
	if name == nil {
		return "-"
	}
	parts := make([]string, 0, 2)
	if strings.TrimSpace(name.GivenName) != "" {
		parts = append(parts, strings.TrimSpace(name.GivenName))
	}
	if strings.TrimSpace(name.FamilyName) != "" {
		parts = append(parts, strings.TrimSpace(name.FamilyName))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, " ")
}

func scimUserPrimaryValue(values *[]IamSCIMUserPhoneNumber) string {
	if values == nil || len(*values) == 0 {
		return "-"
	}
	for _, value := range *values {
		if strings.TrimSpace(value.Value) != "" {
			return strings.TrimSpace(value.Value)
		}
	}
	return "-"
}

func groupMembershipUserID(item IamFlexeraGroupMembership) int {
	if item.User == nil {
		return 0
	}
	return item.User.Id
}

func groupMembershipUserName(item IamFlexeraGroupMembership) string {
	if item.User == nil {
		return "-"
	}
	return userDisplayName(*item.User)
}

func groupMembershipUserEmail(item IamFlexeraGroupMembership) string {
	if item.User == nil {
		return "-"
	}
	return item.User.Email
}

func contractStatus(item IamContract) string {
	if item.Status == nil {
		return "-"
	}
	return strings.TrimSpace(string(*item.Status))
}

func int64PointerValue(value *int64) string {
	if value == nil {
		return "-"
	}
	return fmt.Sprintf("%d", *value)
}

func policyPublishedTemplateLinkName(link *PolicyFlexeraPolicyPublishedTemplateLink) string {
	if link == nil || strings.TrimSpace(link.Name) == "" {
		return "-"
	}
	return strings.TrimSpace(link.Name)
}

func policyCustomCatalogChildVisibility(value *PolicyFlexeraPolicyCustomCatalog) string {
	if value == nil {
		return "-"
	}
	return boolPointerValue(value.ChildOrgsVisibility)
}

func policyPrincipalName(principal PolicyApplicationVndFlexeraPolicyPrincipal) string {
	if strings.TrimSpace(principal.Name) != "" {
		return strings.TrimSpace(principal.Name)
	}
	if principal.Email != nil && strings.TrimSpace(string(*principal.Email)) != "" {
		return strings.TrimSpace(string(*principal.Email))
	}
	if principal.Id > 0 {
		return fmt.Sprintf("%d", principal.Id)
	}
	return "-"
}

func policyAggregateLinkName(link *PolicyFlexeraPolicyPolicyAggregateLink) string {
	if link == nil || strings.TrimSpace(link.Name) == "" {
		return "-"
	}
	return strings.TrimSpace(link.Name)
}

func policyAggregateStatus(value *PolicyFlexeraPolicyPolicyAggregateStatus) string {
	if value == nil {
		return "-"
	}
	return strings.TrimSpace(string(*value))
}

func policyIncidentSeverity(value *PolicyFlexeraPolicyIncidentAggregateSeverity) string {
	if value == nil {
		return "-"
	}
	return strings.TrimSpace(string(*value))
}

func policyIncidentState(value *PolicyFlexeraPolicyIncidentAggregateState) string {
	if value == nil {
		return "-"
	}
	return strings.TrimSpace(string(*value))
}

func policyAppliedPolicyStatus(value *PolicyFlexeraPolicyAppliedPolicyStatus) string {
	if value == nil {
		return "-"
	}
	return strings.TrimSpace(string(*value))
}

func policyAppliedPolicyScope(value *PolicyFlexeraPolicyAppliedPolicyScope) string {
	if value == nil {
		return "-"
	}
	return strings.TrimSpace(string(*value))
}

func policyAppliedPolicyTemplateName(policyTemplate *PolicyFlexeraPolicyPolicyTemplateLink, publishedTemplate *PolicyFlexeraPolicyPublishedTemplateLink) string {
	if policyTemplate != nil && strings.TrimSpace(policyTemplate.Name) != "" {
		return strings.TrimSpace(policyTemplate.Name)
	}
	if publishedTemplate != nil && strings.TrimSpace(publishedTemplate.Name) != "" {
		return strings.TrimSpace(publishedTemplate.Name)
	}
	return "-"
}

func policyActionStatusAppliedPolicyID(link *PolicyFlexeraPolicyActionStatusAppliedPolicy) string {
	if link == nil || strings.TrimSpace(link.Id) == "" {
		return "-"
	}
	return strings.TrimSpace(link.Id)
}

func policyActionStatusIncidentID(link *PolicyFlexeraPolicyActionStatusIncident) string {
	if link == nil || strings.TrimSpace(link.Id) == "" {
		return "-"
	}
	return strings.TrimSpace(link.Id)
}

func policyArchivedIncidentState(value *PolicyRightscaleArchivedIncidentState) string {
	if value == nil {
		return "-"
	}
	return strings.TrimSpace(string(*value))
}

func policyArchivedIncidentSeverity(value *PolicyRightscaleArchivedIncidentSeverity) string {
	if value == nil {
		return "-"
	}
	return strings.TrimSpace(string(*value))
}

func policyArchivedIncidentAppliedPolicyName(link *PolicyFlexeraPolicyAppliedPolicyLink) string {
	if link == nil || strings.TrimSpace(link.Name) == "" {
		return "-"
	}
	return strings.TrimSpace(link.Name)
}

func policyAppliedPolicyLinkName(link *PolicyFlexeraPolicyAppliedPolicyLink) string {
	if link == nil || strings.TrimSpace(link.Name) == "" {
		return "-"
	}
	return strings.TrimSpace(link.Name)
}

func policyTemplateLinkName(link *PolicyFlexeraPolicyPolicyTemplateLink) string {
	if link == nil || strings.TrimSpace(link.Name) == "" {
		return "-"
	}
	return strings.TrimSpace(link.Name)
}

func policyProjectName(project *PolicyProject) string {
	if project == nil || strings.TrimSpace(project.Name) == "" {
		return "-"
	}
	return strings.TrimSpace(project.Name)
}

func policyUnmanagedIncidentState(value *PolicyFlexeraPolicyIncidentAggregateUnmanagedItemState) string {
	if value == nil {
		return "-"
	}
	return strings.TrimSpace(string(*value))
}

func policyUnmanagedIncidentSeverity(value *PolicyFlexeraPolicyIncidentAggregateUnmanagedItemSeverity) string {
	if value == nil {
		return "-"
	}
	return strings.TrimSpace(string(*value))
}

func policyUnmanagedAppliedPolicyStatus(value *PolicyFlexeraPolicyPolicyAggregateUnmanagedItemStatus) string {
	if value == nil {
		return "-"
	}
	return strings.TrimSpace(string(*value))
}
