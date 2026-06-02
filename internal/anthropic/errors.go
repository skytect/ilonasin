package anthropic

type ErrorEnvelope struct {
	Type  string    `json:"type"`
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func Error(message string) ErrorEnvelope {
	return ErrorWithType(message, "invalid_request_error")
}

func ErrorForStatus(status int, message string) ErrorEnvelope {
	switch status {
	case 401, 403:
		return ErrorWithType(message, "authentication_error")
	case 404:
		return ErrorWithType(message, "not_found_error")
	case 429:
		return ErrorWithType(message, "rate_limit_error")
	}
	if status >= 500 {
		return ErrorWithType(message, "api_error")
	}
	return ErrorWithType(message, "invalid_request_error")
}

func ErrorWithType(message, errorType string) ErrorEnvelope {
	return ErrorEnvelope{
		Type:  "error",
		Error: ErrorBody{Type: errorType, Message: message},
	}
}
