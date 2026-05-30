package routing

import (
	"fmt"
	"strings"
)

type ModelAddress struct {
	ProviderInstanceID string
	ProviderModelID    string
}

func ParseModelAddress(model string) (ModelAddress, error) {
	provider, providerModel, ok := strings.Cut(model, "/")
	if !ok || provider == "" || providerModel == "" {
		return ModelAddress{}, fmt.Errorf("model must be addressed as <provider_instance_id>/<provider_model_id>")
	}
	return ModelAddress{ProviderInstanceID: provider, ProviderModelID: providerModel}, nil
}
