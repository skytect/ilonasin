package tui

import "strings"

func (m Model) writeHelp(b *strings.Builder) {
	b.WriteString("Guidance\n")
	b.WriteString("- tab / shift+tab switch sections\n")
	b.WriteString("- 1-4 jump to api, providers, usage, logs\n")
	b.WriteString("- up/down or j/k select local tokens on api, scroll elsewhere\n")
	b.WriteString("- pgup/pgdown, ctrl+u/ctrl+d, home/end scroll content\n")
	b.WriteString("- n create local token on api\n")
	b.WriteString("- d disable selected local token on api\n")
	b.WriteString("- a add upstream API key on providers\n")
	b.WriteString("- x disable first enabled upstream credential on providers\n")
	b.WriteString("- l login or relogin OAuth on providers\n")
	b.WriteString("- o select OAuth account on providers\n")
	b.WriteString("- r refresh selected OAuth account on providers\n")
	b.WriteString("- f/F enable or disable first credential group fallback on providers\n")
	b.WriteString("- u refresh subscription usage on usage\n")
	b.WriteString("- p prune telemetry older than 30 days on logs\n")
	b.WriteString("- esc clears transient messages or cancels OAuth login\n")
	b.WriteString("- q quits\n")
	b.WriteString("\nPrivacy\n")
	b.WriteString("The TUI renders snapshot metadata and redacted display values only. It does not render prompts, completions, request bodies, response bodies, raw streams, tool arguments, tool results, provider payloads, provider request IDs, full local tokens, or full provider account IDs.\n")
}
