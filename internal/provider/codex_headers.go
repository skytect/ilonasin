package provider

import (
	"context"
	"net/http"
	"runtime"
)

const (
	// Mirrors OpenAI Codex rust-v0.135.0:
	// codex-rs/login/src/auth/default_client.rs DEFAULT_ORIGINATOR.
	CodexOriginator = "codex_cli_rs"
)

func (a HTTPChatAdapter) addCodexRequestHeaders(ctx context.Context, req *http.Request, token, accountID string, fedRAMP bool) {
	version := a.codexClientVersion(ctx)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("originator", CodexOriginator)
	req.Header.Set("User-Agent", codexUserAgent(version))
	if accountID != "" {
		req.Header.Set("ChatGPT-Account-ID", accountID)
	}
	if fedRAMP {
		req.Header.Set("X-OpenAI-Fedramp", "true")
	}
}

func codexUserAgent(version string) string {
	// Mirrors the stable prefix from OpenAI Codex
	// codex-rs/login/src/auth/default_client.rs get_codex_user_agent.
	return CodexOriginator + "/" + version + " (" + runtime.GOOS + "; " + runtime.GOARCH + ")"
}
