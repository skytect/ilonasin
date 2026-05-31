package provider

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
)

func openRouterCostMicrounitsFromChatCompletion(body []byte) int64 {
	var payload struct {
		Usage json.RawMessage `json:"usage"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0
	}
	return openRouterCostMicrounitsFromUsage(payload.Usage)
}

func openRouterCostMicrounitsFromStreamChunk(body []byte) int64 {
	var payload struct {
		Usage json.RawMessage `json:"usage"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0
	}
	return openRouterCostMicrounitsFromUsage(payload.Usage)
}

func openRouterCostMicrounitsFromUsage(rawUsage json.RawMessage) int64 {
	rawUsage = bytes.TrimSpace(rawUsage)
	if len(rawUsage) == 0 || bytes.Equal(rawUsage, []byte("null")) {
		return 0
	}
	var usage map[string]json.RawMessage
	if err := json.Unmarshal(rawUsage, &usage); err != nil {
		return 0
	}
	rawCost, ok := usage["cost"]
	if !ok {
		return 0
	}
	return openRouterCostMicrounitsFromRawCost(rawCost)
}

func openRouterCostMicrounitsFromRawCost(rawCost json.RawMessage) int64 {
	rawCost = bytes.TrimSpace(rawCost)
	if len(rawCost) == 0 || bytes.Equal(rawCost, []byte("null")) {
		return 0
	}
	if (rawCost[0] < '0' || rawCost[0] > '9') && rawCost[0] != '-' {
		return 0
	}
	dec := json.NewDecoder(bytes.NewReader(rawCost))
	dec.UseNumber()
	var cost json.Number
	if err := dec.Decode(&cost); err != nil {
		return 0
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return 0
	}
	return openRouterCreditMicrounits(cost.String())
}

func openRouterCreditMicrounits(value string) int64 {
	if value == "" || len(value) > 128 || value[0] == '-' {
		return 0
	}
	mantissa, exponent, ok := strings.Cut(value, "e")
	if !ok {
		mantissa, exponent, ok = strings.Cut(value, "E")
	}
	exp := 0
	if ok {
		if exponent == "" || len(exponent) > 4 {
			return 0
		}
		parsed, err := strconv.Atoi(exponent)
		if err != nil {
			return 0
		}
		exp = parsed
	}
	digits, fractionDigits, ok := decimalDigits(mantissa)
	if !ok {
		return 0
	}
	digits = strings.TrimLeft(digits, "0")
	if digits == "" {
		return 0
	}
	decimalExp := exp - fractionDigits + 6
	if decimalExp > 19 || decimalExp < -128 {
		return 0
	}
	valueInt, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		return 0
	}
	if decimalExp >= 0 {
		valueInt.Mul(valueInt, pow10(decimalExp))
		if valueInt.Cmp(new(big.Int).SetInt64(math.MaxInt64)) > 0 {
			return 0
		}
		return valueInt.Int64()
	}
	divisor := pow10(-decimalExp)
	quotient, remainder := new(big.Int).QuoRem(valueInt, divisor, new(big.Int))
	if new(big.Int).Mul(remainder, big.NewInt(2)).Cmp(divisor) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if quotient.Cmp(new(big.Int).SetInt64(math.MaxInt64)) > 0 {
		return 0
	}
	return quotient.Int64()
}

func decimalDigits(value string) (string, int, bool) {
	if value == "" {
		return "", 0, false
	}
	var digits strings.Builder
	fractionDigits := 0
	seenDot := false
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
			if seenDot {
				fractionDigits++
			}
		case r == '.':
			if seenDot {
				return "", 0, false
			}
			seenDot = true
		default:
			return "", 0, false
		}
	}
	if digits.Len() == 0 {
		return "", 0, false
	}
	return digits.String(), fractionDigits, true
}

func pow10(exp int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exp)), nil)
}

func openRouterCapabilityFlags(item map[string]any) string {
	flags := map[string]bool{"chat": true}
	params, _ := item["supported_parameters"].([]any)
	for _, raw := range params {
		param, ok := raw.(string)
		if !ok {
			continue
		}
		switch param {
		case "temperature", "top_p", "frequency_penalty", "presence_penalty", "stop":
			flags["sampling"] = true
		case "top_k", "min_p", "top_a", "repetition_penalty", "seed":
			flags["advanced_sampling"] = true
		case "response_format":
			flags["json_object"] = true
		case "tools", "tool_choice":
			flags["tools"] = true
		case "parallel_tool_calls":
			flags["parallel_tool_calls"] = true
		case "prediction":
			flags["prediction"] = true
		case "logprobs", "top_logprobs":
			flags["logprobs"] = true
		case "logit_bias":
			flags["logit_bias"] = true
		case "reasoning":
			flags["reasoning"] = true
		case "stream":
			flags["stream"] = true
		case "user":
			flags["user"] = true
		case "service_tier":
			flags["service_tier"] = true
		case "session_id":
			flags["session_id"] = true
		case "metadata":
			flags["metadata"] = true
		case "models":
			flags["model_fallbacks"] = true
		case "cache_control":
			flags["cache_control"] = true
		}
	}
	out := make([]string, 0, len(flags))
	for flag := range flags {
		out = append(out, flag)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}
