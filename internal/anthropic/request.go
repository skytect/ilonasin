package anthropic

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

func DecodeRequest(r io.Reader) (Request, error) {
	return decodeRequest(r, true)
}

func DecodeCountTokensRequest(r io.Reader) (Request, error) {
	return decodeRequest(r, false)
}

func decodeRequest(r io.Reader, requireMaxTokens bool) (Request, error) {
	dec := json.NewDecoder(r)
	dec.UseNumber()
	var raw map[string]json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return Request{}, fmt.Errorf("invalid request JSON: %w", err)
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return Request{}, errors.New("request body must contain a single JSON object")
	}
	if key, ok := firstUnsupportedAnthropicField(raw, "model", "max_tokens", "messages", "system", "stream", "temperature", "top_p", "top_k", "stop_sequences", "tools", "tool_choice", "metadata", "cache_control", "thinking", "context_management", "output_config"); ok {
		return Request{}, fmt.Errorf("%s is not supported", key)
	}

	req := Request{}
	if err := decodeRequiredString(raw, "model", &req.Model); err != nil {
		return Request{}, err
	}
	if requireMaxTokens {
		if err := decodePositiveInt(raw, "max_tokens", &req.MaxTokens); err != nil {
			return Request{}, err
		}
	} else if _, ok := raw["max_tokens"]; ok {
		if err := decodePositiveInt(raw, "max_tokens", &req.MaxTokens); err != nil {
			return Request{}, err
		}
	}
	if err := decodeMessages(raw["messages"], &req.Messages); err != nil {
		return Request{}, err
	}
	if system, ok := raw["system"]; ok {
		blocks, err := decodeSystem(system)
		if err != nil {
			return Request{}, err
		}
		req.System = blocks
	}
	if rawStream, ok := raw["stream"]; ok {
		if err := json.Unmarshal(rawStream, &req.Stream); err != nil {
			return Request{}, errors.New("stream must be a boolean")
		}
	}
	if err := decodeOptionalFloat(raw, "temperature", &req.Temperature); err != nil {
		return Request{}, err
	}
	if err := decodeOptionalFloat(raw, "top_p", &req.TopP); err != nil {
		return Request{}, err
	}
	if rawTopK, ok := raw["top_k"]; ok {
		var n json.Number
		if err := json.Unmarshal(rawTopK, &n); err != nil {
			return Request{}, errors.New("top_k must be a number")
		}
		req.TopK = &n
	}
	if rawStop, ok := raw["stop_sequences"]; ok {
		if err := json.Unmarshal(rawStop, &req.StopSequences); err != nil {
			return Request{}, errors.New("stop_sequences must be an array of strings")
		}
		for _, value := range req.StopSequences {
			if value == "" {
				return Request{}, errors.New("stop_sequences must not contain empty strings")
			}
		}
	}
	if rawTools, ok := raw["tools"]; ok {
		tools, err := decodeTools(rawTools)
		if err != nil {
			return Request{}, err
		}
		req.Tools = tools
	}
	if rawChoice, ok := raw["tool_choice"]; ok {
		choice, err := decodeToolChoice(rawChoice)
		if err != nil {
			return Request{}, err
		}
		req.ToolChoice = choice
	}
	if rawMetadata, ok := raw["metadata"]; ok {
		var metadata map[string]any
		if err := json.Unmarshal(rawMetadata, &metadata); err != nil {
			return Request{}, errors.New("metadata must be an object")
		}
		req.Metadata = metadata
	}
	if rawCacheControl, ok := raw["cache_control"]; ok {
		cacheControl, err := decodeCacheControl(rawCacheControl, "cache_control")
		if err != nil {
			return Request{}, err
		}
		req.CacheControl = cacheControl
	}
	if rawThinking, ok := raw["thinking"]; ok {
		thinking, err := decodeThinking(rawThinking)
		if err != nil {
			return Request{}, err
		}
		req.Thinking = thinking
	}
	if rawContext, ok := raw["context_management"]; ok {
		context, err := decodeContextManagement(rawContext)
		if err != nil {
			return Request{}, err
		}
		req.Context = context
	}
	if rawOutput, ok := raw["output_config"]; ok {
		output, err := decodeOutputConfig(rawOutput)
		if err != nil {
			return Request{}, err
		}
		req.OutputConfig = output
	}
	return req, nil
}
