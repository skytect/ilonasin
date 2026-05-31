package provider

import "errors"

func validateDeepSeekOptions(raw any) error {
	opts, ok := raw.(map[string]any)
	if !ok || opts == nil {
		return errors.New("provider_options.deepseek must be an object")
	}
	if len(opts) == 0 {
		return errors.New("provider_options.deepseek must not be empty")
	}
	for key, value := range opts {
		switch key {
		case "thinking":
			thinking, ok := value.(map[string]any)
			if !ok || thinking == nil {
				return errors.New("provider_options.deepseek.thinking must be an object")
			}
			if len(thinking) != 1 {
				return errors.New("provider_options.deepseek.thinking only supports type")
			}
			typ, ok := thinking["type"].(string)
			if !ok {
				return errors.New("provider_options.deepseek.thinking.type must be a string")
			}
			if typ != "enabled" && typ != "disabled" {
				return errors.New("provider_options.deepseek.thinking.type is unsupported")
			}
		case "reasoning_effort":
			effort, ok := value.(string)
			if !ok {
				return errors.New("provider_options.deepseek.reasoning_effort must be a string")
			}
			if effort != "high" && effort != "max" {
				return errors.New("provider_options.deepseek.reasoning_effort is unsupported")
			}
		case "user_id":
			userID, ok := value.(string)
			if !ok {
				return errors.New("provider_options.deepseek.user_id must be a string")
			}
			if !isDeepSeekUserID(userID) {
				return errors.New("provider_options.deepseek.user_id is invalid")
			}
		default:
			return errors.New("provider_options.deepseek contains an unsupported field")
		}
	}
	return nil
}

func isDeepSeekUserID(value string) bool {
	if value == "" || len(value) > 512 {
		return false
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			return false
		}
	}
	return true
}
