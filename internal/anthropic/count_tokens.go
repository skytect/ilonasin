package anthropic

import (
	"encoding/json"
	"unicode/utf8"
)

func CountInputTokens(req Request) CountTokensResponse {
	chars := 0
	structural := 8
	for _, block := range req.System {
		chars += blockTextSize(block)
		structural += 4
	}
	for _, msg := range req.Messages {
		chars += len(msg.Role)
		structural += 6
		for _, block := range msg.Content {
			chars += blockTextSize(block)
			structural += 4
		}
	}
	for _, tool := range req.Tools {
		chars += len(tool.Name) + len(tool.Description) + len(tool.InputSchema)
		structural += 12
	}
	tokens := structural + (chars+3)/4
	if tokens < 1 {
		tokens = 1
	}
	return CountTokensResponse{InputTokens: tokens}
}

func blockTextSize(block ContentBlock) int {
	switch block.Type {
	case "text":
		return utf8.RuneCountInString(block.Text)
	case "image":
		return utf8.RuneCountInString(block.SourceURL)
	case "tool_use":
		return utf8.RuneCountInString(block.ToolUseID) +
			utf8.RuneCountInString(block.ToolName) +
			jsonSize(block.ToolInput)
	case "tool_result":
		return utf8.RuneCountInString(block.ToolUseID) +
			utf8.RuneCountInString(block.ToolContent)
	default:
		return 0
	}
}

func jsonSize(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	return len(raw)
}
