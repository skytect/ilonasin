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
		case "format":
			format, ok := value.(map[string]any)
			if !ok || format == nil {
				return errors.New("provider_options.codex.format must be an object")
			}
			if err := validateCodexTextFormat(format); err != nil {
				return err
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

func validateCodexTextFormat(format map[string]any) error {
	if len(format) < 3 || len(format) > 4 {
		return errors.New("provider_options.codex.format only supports type, name, schema, and strict")
	}
	typ, ok := format["type"].(string)
	if !ok {
		return errors.New("provider_options.codex.format.type must be a string")
	}
	if typ != "json_schema" {
		return errors.New("provider_options.codex.format.type is unsupported")
	}
	name, ok := format["name"].(string)
	if !ok {
		return errors.New("provider_options.codex.format.name must be a string")
	}
	if name == "" || len(name) > 64 {
		return errors.New("provider_options.codex.format.name is invalid")
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			return errors.New("provider_options.codex.format.name is invalid")
		}
	}
	schema, ok := format["schema"].(map[string]any)
	if !ok || schema == nil {
		return errors.New("provider_options.codex.format.schema must be an object")
	}
	if len(schema) == 0 {
		return errors.New("provider_options.codex.format.schema must not be empty")
	}
	for key, value := range format {
		switch key {
		case "type", "name", "schema":
		case "strict":
			if _, ok := value.(bool); !ok {
				return errors.New("provider_options.codex.format.strict must be a boolean")
			}
		default:
			return errors.New("provider_options.codex.format contains an unsupported field")
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
