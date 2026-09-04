package routing

import (
	"fmt"
	"strings"
)

type ModelAddress struct {
	ProviderInstanceID string
	ProviderModelID    string
	AccountSelector    string
}

// RequestedModel retains the routing selector for metadata and diagnostics.
// ProviderModelID alone is always sent to the upstream model API.
func (a ModelAddress) RequestedModel() string {
	if a.AccountSelector != "" {
		return a.ProviderModelID + "/" + a.AccountSelector
	}
	return a.ProviderModelID
}

// SplitAccountSelector reserves a final /daybreak-<name> segment for account
// routing. Names are discovered from upstream catalogs, not an allowlist.
func SplitAccountSelector(model string) (string, string, error) {
	i := strings.LastIndexByte(model, '/')
	if i < 0 || !strings.HasPrefix(model[i+1:], "daybreak-") {
		return model, "", nil
	}
	selector := model[i+1:]
	name := strings.TrimPrefix(selector, "daybreak-")
	if strings.TrimSpace(model[:i]) == "" || name == "" {
		return "", "", fmt.Errorf("account selector requires a model and a nonempty daybreak name")
	}
	for _, segment := range strings.Split(model[:i], "/") {
		if strings.HasPrefix(segment, "daybreak-") {
			return "", "", fmt.Errorf("model accepts only one final daybreak account selector")
		}
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			return "", "", fmt.Errorf("daybreak account selector must contain lowercase letters, digits or hyphens")
		}
	}
	return model[:i], selector, nil
}

func ParseModelAddress(model string) (ModelAddress, error) {
	model, selector, err := SplitAccountSelector(model)
	if err != nil {
		return ModelAddress{}, err
	}
	provider, providerModel, ok := strings.Cut(model, "/")
	if !ok || provider == "" || providerModel == "" {
		return ModelAddress{}, fmt.Errorf("model must be addressed as <provider_instance_id>/<provider_model_id>")
	}
	return ModelAddress{ProviderInstanceID: provider, ProviderModelID: providerModel, AccountSelector: selector}, nil
}
