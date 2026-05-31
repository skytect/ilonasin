package provider

import (
	"net/http"
	"runtime"
)

const (
	// Mirrors OpenAI Codex rust-v0.135.0:
	// codex-rs/login/src/auth/default_client.rs DEFAULT_ORIGINATOR.
	CodexOriginator = "codex_cli_rs"
	// Mirrors OpenAI Codex rust-v0.135.0:
	// codex-rs/models-manager/src/lib.rs passes the Cargo package version to /models.
	CodexClientVersion = "0.135.0"
)

func addCodexRequestHeaders(req *http.Request, token, accountID string, fedRAMP bool) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("originator", CodexOriginator)
	req.Header.Set("User-Agent", codexUserAgent())
	if accountID != "" {
		req.Header.Set("ChatGPT-Account-ID", accountID)
	}
	if fedRAMP {
		req.Header.Set("X-OpenAI-Fedramp", "true")
	}
}

func codexUserAgent() string {
	// Mirrors the stable prefix from OpenAI Codex rust-v0.135.0
	// codex-rs/login/src/auth/default_client.rs get_codex_user_agent.
	return CodexOriginator + "/" + CodexClientVersion + " (" + runtime.GOOS + "; " + runtime.GOARCH + ")"
}
