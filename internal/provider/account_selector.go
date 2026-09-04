package provider

import "strings"

// AccountSelectorFromModelID recognizes upstream account-cohort markers. They
// describe account eligibility, not a replacement for the requested model.
func AccountSelectorFromModelID(modelID string) (string, bool) {
	name := strings.TrimSuffix(strings.TrimPrefix(modelID, "gpt-daybreak-"), "-latest")
	if name == "" || modelID != "gpt-daybreak-"+name+"-latest" {
		return "", false
	}
	return "daybreak-" + name, true
}

// ModelCatalogRequirements are exact IDs that the same credential must
// advertise. No model substitution or cross-account entitlement union applies.
func ModelCatalogRequirements(modelID, selector string) []string {
	if selector == "" {
		return []string{modelID}
	}
	return []string{modelID, "gpt-" + selector + "-latest"}
}
