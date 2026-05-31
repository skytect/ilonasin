package provider

import "errors"

func validateCodexOptions(raw any) error {
	opts, ok := raw.(map[string]any)
	if !ok || opts == nil {
		return errors.New("provider_options.codex must be an object")
	}
	if len(opts) == 0 {
		return errors.New("provider_options.codex must not be empty")
	}
	for key, value := range opts {
		switch key {
		case "reasoning":
			if err := validateCodexReasoning(value); err != nil {
				return err
			}
		case "verbosity":
			verbosity, ok := value.(string)
			if !ok {
				return errors.New("provider_options.codex.verbosity must be a string")
			}
			if verbosity != "low" && verbosity != "medium" && verbosity != "high" {
				return errors.New("provider_options.codex.verbosity is unsupported")
			}
		case "service_tier":
			tier, ok := value.(string)
			if !ok {
				return errors.New("provider_options.codex.service_tier must be a string")
			}
			if tier != "default" && tier != "priority" && tier != "flex" && tier != "fast" {
				return errors.New("provider_options.codex.service_tier is unsupported")
			}
		default:
			return errors.New("provider_options.codex contains an unsupported field")
		}
	}
	return nil
}

func validateCodexReasoning(raw any) error {
	reasoning, ok := raw.(map[string]any)
	if !ok || reasoning == nil {
		return errors.New("provider_options.codex.reasoning must be an object")
	}
	if len(reasoning) == 0 {
		return errors.New("provider_options.codex.reasoning must not be empty")
	}
	for key, value := range reasoning {
		switch key {
		case "effort":
			effort, ok := value.(string)
			if !ok {
				return errors.New("provider_options.codex.reasoning.effort must be a string")
			}
			switch effort {
			case "none", "minimal", "low", "medium", "high", "xhigh":
			default:
				return errors.New("provider_options.codex.reasoning.effort is unsupported")
			}
		case "summary":
			summary, ok := value.(string)
			if !ok {
				return errors.New("provider_options.codex.reasoning.summary must be a string")
			}
			switch summary {
			case "auto", "concise", "detailed", "none":
			default:
				return errors.New("provider_options.codex.reasoning.summary is unsupported")
			}
		default:
			return errors.New("provider_options.codex.reasoning contains an unsupported field")
		}
	}
	return nil
}
