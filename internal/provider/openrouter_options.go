package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

func validateOpenRouterJSONSchemaResponseFormat(format map[string]any) error {
	if len(format) != 2 {
		return errors.New("response_format only supports type and json_schema")
	}
	rawSchema, ok := format["json_schema"]
	if !ok {
		return errors.New("response_format.json_schema is required")
	}
	schema, ok := rawSchema.(map[string]any)
	if !ok || schema == nil {
		return errors.New("response_format.json_schema must be an object")
	}
	if len(schema) == 0 {
		return errors.New("response_format.json_schema must not be empty")
	}
	name, ok := schema["name"].(string)
	if !ok {
		return errors.New("response_format.json_schema.name must be a string")
	}
	if name == "" || len(name) > 64 || !isOpenRouterJSONSchemaName(name) {
		return errors.New("response_format.json_schema.name is invalid")
	}
	rawBody, ok := schema["schema"]
	if !ok {
		return errors.New("response_format.json_schema.schema is required")
	}
	body, ok := rawBody.(map[string]any)
	if !ok || body == nil {
		return errors.New("response_format.json_schema.schema must be an object")
	}
	_ = body
	for key, value := range schema {
		switch key {
		case "name", "schema":
		case "strict":
			if _, ok := value.(bool); !ok {
				return errors.New("response_format.json_schema.strict must be a boolean")
			}
		case "description":
			if _, ok := value.(string); !ok {
				return errors.New("response_format.json_schema.description must be a string")
			}
		default:
			return errors.New("response_format.json_schema contains an unsupported field")
		}
	}
	return nil
}

func isOpenRouterJSONSchemaName(value string) bool {
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

func validateOpenRouterOptions(raw any) error {
	opts, ok := raw.(map[string]any)
	if !ok || opts == nil {
		return errors.New("provider_options.openrouter must be an object")
	}
	if len(opts) == 0 {
		return errors.New("provider_options.openrouter must not be empty")
	}
	for key, value := range opts {
		switch key {
		case "reasoning":
			if err := validateOpenRouterReasoning(value); err != nil {
				return err
			}
		case "models":
			if err := validateOpenRouterModelList(value); err != nil {
				return err
			}
		case "cache_control":
			if err := validateOpenRouterCacheControl(value); err != nil {
				return err
			}
		case "provider":
			if err := validateOpenRouterProvider(value); err != nil {
				return err
			}
		default:
			return errors.New("provider_options.openrouter contains an unsupported field")
		}
	}
	return nil
}

func validateOpenRouterReasoning(raw any) error {
	reasoning, ok := raw.(map[string]any)
	if !ok || reasoning == nil {
		return errors.New("provider_options.openrouter.reasoning must be an object")
	}
	if len(reasoning) == 0 {
		return errors.New("provider_options.openrouter.reasoning must not be empty")
	}
	hasEffort := false
	hasMaxTokens := false
	for key, value := range reasoning {
		switch key {
		case "effort":
			effort, ok := value.(string)
			if !ok {
				return errors.New("provider_options.openrouter.reasoning.effort must be a string")
			}
			if !isOpenRouterReasoningEffort(effort) {
				return errors.New("provider_options.openrouter.reasoning.effort is unsupported")
			}
			hasEffort = true
		case "max_tokens":
			if !isPositiveJSONInteger(value) {
				return errors.New("provider_options.openrouter.reasoning.max_tokens must be a positive integer")
			}
			hasMaxTokens = true
		case "exclude", "enabled":
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("provider_options.openrouter.reasoning.%s must be a boolean", key)
			}
		default:
			return errors.New("provider_options.openrouter.reasoning contains an unsupported field")
		}
	}
	if hasEffort && hasMaxTokens {
		return errors.New("provider_options.openrouter.reasoning.effort and max_tokens are mutually exclusive")
	}
	return nil
}

func validateOpenRouterModelList(raw any) error {
	values, ok := raw.([]any)
	if !ok || len(values) == 0 || len(values) > 32 {
		return errors.New("provider_options.openrouter.models must be a non-empty array of up to 32 model slugs")
	}
	seen := map[string]bool{}
	for _, rawValue := range values {
		value, ok := rawValue.(string)
		if !ok || !isOpenRouterModelSlug(value) {
			return errors.New("provider_options.openrouter.models must contain only model slug strings")
		}
		if seen[value] {
			return errors.New("provider_options.openrouter.models must not contain duplicate model slugs")
		}
		seen[value] = true
	}
	return nil
}

func isOpenRouterModelSlug(value string) bool {
	if value == "" || len(value) > 256 {
		return false
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-' || r == '.' || r == '/' || r == ':' || r == '~':
		default:
			return false
		}
	}
	return true
}

func validateOpenRouterCacheControl(raw any) error {
	cacheControl, ok := raw.(map[string]any)
	if !ok || cacheControl == nil {
		return errors.New("provider_options.openrouter.cache_control must be an object")
	}
	if len(cacheControl) == 0 {
		return errors.New("provider_options.openrouter.cache_control must not be empty")
	}
	typ, ok := cacheControl["type"].(string)
	if !ok {
		return errors.New("provider_options.openrouter.cache_control.type must be a string")
	}
	if typ != "ephemeral" {
		return errors.New("provider_options.openrouter.cache_control.type is unsupported")
	}
	for key, value := range cacheControl {
		switch key {
		case "type":
		case "ttl":
			ttl, ok := value.(string)
			if !ok {
				return errors.New("provider_options.openrouter.cache_control.ttl must be a string")
			}
			if ttl != "5m" && ttl != "1h" {
				return errors.New("provider_options.openrouter.cache_control.ttl is unsupported")
			}
		default:
			return errors.New("provider_options.openrouter.cache_control contains an unsupported field")
		}
	}
	return nil
}

func validateOpenRouterProvider(raw any) error {
	provider, ok := raw.(map[string]any)
	if !ok || provider == nil {
		return errors.New("provider_options.openrouter.provider must be an object")
	}
	if len(provider) == 0 {
		return errors.New("provider_options.openrouter.provider must not be empty")
	}
	hasOrder := false
	hasSort := false
	for key, value := range provider {
		switch key {
		case "require_parameters":
			if _, ok := value.(bool); !ok {
				return errors.New("provider_options.openrouter.provider.require_parameters must be a boolean")
			}
		case "allow_fallbacks":
			if _, ok := value.(bool); !ok {
				return errors.New("provider_options.openrouter.provider.allow_fallbacks must be a boolean")
			}
		case "order", "only", "ignore":
			if err := validateOpenRouterProviderSlugList(key, value); err != nil {
				return err
			}
			if key == "order" {
				hasOrder = true
			}
		case "quantizations":
			if err := validateOpenRouterQuantizations(value); err != nil {
				return err
			}
		case "max_price":
			if err := validateOpenRouterMaxPrice(value); err != nil {
				return err
			}
		case "preferred_max_latency", "preferred_min_throughput":
			if err := validateOpenRouterPerformancePreference(key, value); err != nil {
				return err
			}
		case "enforce_distillable_text":
			if _, ok := value.(bool); !ok {
				return errors.New("provider_options.openrouter.provider.enforce_distillable_text must be a boolean")
			}
		case "sort":
			if err := validateOpenRouterProviderSort(value); err != nil {
				return err
			}
			hasSort = true
		case "data_collection":
			collection, ok := value.(string)
			if !ok {
				return errors.New("provider_options.openrouter.provider.data_collection must be a string")
			}
			if collection != "deny" {
				return errors.New("provider_options.openrouter.provider.data_collection is unsupported")
			}
		case "zdr":
			zdr, ok := value.(bool)
			if !ok {
				return errors.New("provider_options.openrouter.provider.zdr must be a boolean")
			}
			if !zdr {
				return errors.New("provider_options.openrouter.provider.zdr is unsupported")
			}
		default:
			return errors.New("provider_options.openrouter.provider contains an unsupported field")
		}
	}
	if hasOrder && hasSort {
		return errors.New("provider_options.openrouter.provider.sort is unsupported when order is specified")
	}
	return nil
}

func validateOpenRouterProviderSlugList(field string, raw any) error {
	values, ok := raw.([]any)
	if !ok || len(values) == 0 || len(values) > 32 {
		return fmt.Errorf("provider_options.openrouter.provider.%s must be a non-empty array of up to 32 provider slugs", field)
	}
	seen := map[string]bool{}
	for _, rawValue := range values {
		value, ok := rawValue.(string)
		if !ok || !isOpenRouterProviderSlug(value) {
			return fmt.Errorf("provider_options.openrouter.provider.%s must contain only provider slug strings", field)
		}
		if seen[value] {
			return fmt.Errorf("provider_options.openrouter.provider.%s must not contain duplicate provider slugs", field)
		}
		seen[value] = true
	}
	return nil
}

func isOpenRouterProviderSlug(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-' || r == '.' || r == '/':
		default:
			return false
		}
	}
	return true
}

func validateOpenRouterQuantizations(raw any) error {
	values, ok := raw.([]any)
	if !ok || len(values) == 0 || len(values) > 16 {
		return errors.New("provider_options.openrouter.provider.quantizations must be a non-empty array of up to 16 quantization strings")
	}
	seen := map[string]bool{}
	for _, rawValue := range values {
		value, ok := rawValue.(string)
		if !ok || !isOpenRouterQuantization(value) {
			return errors.New("provider_options.openrouter.provider.quantizations contains an unsupported quantization")
		}
		if seen[value] {
			return errors.New("provider_options.openrouter.provider.quantizations must not contain duplicates")
		}
		seen[value] = true
	}
	return nil
}

func isOpenRouterQuantization(value string) bool {
	switch value {
	case "int4", "int8", "fp4", "fp6", "fp8", "fp16", "bf16", "fp32", "unknown":
		return true
	default:
		return false
	}
}

func validateOpenRouterProviderSort(raw any) error {
	switch value := raw.(type) {
	case string:
		if !isOpenRouterProviderSortCriterion(value) {
			return errors.New("provider_options.openrouter.provider.sort is unsupported")
		}
		return nil
	case map[string]any:
		if len(value) == 0 {
			return errors.New("provider_options.openrouter.provider.sort must be a non-empty object")
		}
		for key, rawField := range value {
			switch key {
			case "by":
				by, ok := rawField.(string)
				if !ok || !isOpenRouterProviderSortCriterion(by) {
					return errors.New("provider_options.openrouter.provider.sort.by is unsupported")
				}
			case "partition":
				partition, ok := rawField.(string)
				if !ok || !isOpenRouterProviderSortPartition(partition) {
					return errors.New("provider_options.openrouter.provider.sort.partition is unsupported")
				}
			default:
				return errors.New("provider_options.openrouter.provider.sort contains an unsupported field")
			}
		}
		return nil
	default:
		return errors.New("provider_options.openrouter.provider.sort must be a string or object")
	}
}

func isOpenRouterProviderSortCriterion(value string) bool {
	switch value {
	case "price", "throughput", "latency", "exacto":
		return true
	default:
		return false
	}
}

func isOpenRouterProviderSortPartition(value string) bool {
	switch value {
	case "model", "none":
		return true
	default:
		return false
	}
}

func validateOpenRouterMaxPrice(raw any) error {
	prices, ok := raw.(map[string]any)
	if !ok || len(prices) == 0 {
		return errors.New("provider_options.openrouter.provider.max_price must be a non-empty object")
	}
	for key, value := range prices {
		switch key {
		case "prompt", "completion", "request", "image", "audio":
		default:
			return errors.New("provider_options.openrouter.provider.max_price contains an unsupported field")
		}
		num, ok := value.(json.Number)
		if !ok || !isOpenRouterMaxPrice(num) {
			return errors.New("provider_options.openrouter.provider.max_price values must be numbers between 0 and 1000000")
		}
	}
	return nil
}

func validateOpenRouterPerformancePreference(field string, raw any) error {
	switch value := raw.(type) {
	case json.Number:
		if !isOpenRouterPositivePreferenceNumber(value) {
			return fmt.Errorf("provider_options.openrouter.provider.%s must be a number between 0 and 1000000", field)
		}
		return nil
	case map[string]any:
		if len(value) == 0 {
			return fmt.Errorf("provider_options.openrouter.provider.%s must be a non-empty object", field)
		}
		for key, rawField := range value {
			switch key {
			case "p50", "p75", "p90", "p99":
			default:
				return fmt.Errorf("provider_options.openrouter.provider.%s contains an unsupported field", field)
			}
			num, ok := rawField.(json.Number)
			if !ok || !isOpenRouterPositivePreferenceNumber(num) {
				return fmt.Errorf("provider_options.openrouter.provider.%s values must be numbers between 0 and 1000000", field)
			}
		}
		return nil
	default:
		return fmt.Errorf("provider_options.openrouter.provider.%s must be a number or object", field)
	}
}

func isOpenRouterPositivePreferenceNumber(num json.Number) bool {
	if !safeOpenRouterBoundedNumberToken(num.String()) {
		return false
	}
	precise, ok := new(big.Rat).SetString(num.String())
	if !ok || precise.Cmp(big.NewRat(0, 1)) <= 0 || precise.Cmp(big.NewRat(1000000, 1)) > 0 {
		return false
	}
	value, err := num.Float64()
	return err == nil && !math.IsInf(value, 0) && !math.IsNaN(value)
}

func isOpenRouterMaxPrice(num json.Number) bool {
	if !safeOpenRouterBoundedNumberToken(num.String()) {
		return false
	}
	precise, ok := new(big.Rat).SetString(num.String())
	if !ok || precise.Cmp(big.NewRat(0, 1)) < 0 || precise.Cmp(big.NewRat(1000000, 1)) > 0 {
		return false
	}
	value, err := num.Float64()
	return err == nil && !math.IsInf(value, 0) && !math.IsNaN(value)
}

func safeOpenRouterBoundedNumberToken(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	_, exponent, ok := strings.Cut(value, "e")
	if !ok {
		_, exponent, ok = strings.Cut(value, "E")
	}
	if !ok {
		return true
	}
	if exponent == "" || len(exponent) > 5 {
		return false
	}
	parsed, err := strconv.Atoi(exponent)
	return err == nil && parsed >= -1024 && parsed <= 1024
}

func isOpenRouterReasoningEffort(value string) bool {
	switch value {
	case "xhigh", "high", "medium", "low", "minimal", "none":
		return true
	default:
		return false
	}
}

func isPositiveJSONInteger(value any) bool {
	num, ok := value.(float64)
	return ok && num > 0 && num <= math.MaxInt64 && math.Trunc(num) == num
}
