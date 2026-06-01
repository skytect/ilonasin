package tui

import "strings"

func (m Model) writeHelp(b *strings.Builder) {
	b.WriteString("Keys\n")
	b.WriteString("- tab / shift+tab switch tabs\n")
	b.WriteString("- 1-4 jump to overview, accounts, observability, help\n")
	b.WriteString("- up/down or j/k scroll content outside accounts\n")
	b.WriteString("- up/down or j/k select local token on accounts\n")
	b.WriteString("- pgup/pgdown, ctrl+u/ctrl+d, home/end scroll content\n")
	b.WriteString("- n create local token on accounts\n")
	b.WriteString("- a add API key on accounts\n")
	b.WriteString("- d disable selected local token on accounts\n")
	b.WriteString("- x disable first enabled API key credential on accounts\n")
	b.WriteString("- l login or relogin OAuth on accounts\n")
	b.WriteString("- o select OAuth account on accounts\n")
	b.WriteString("- r refresh selected OAuth account on accounts\n")
	b.WriteString("- f/F enable or disable first credential group fallback on accounts\n")
	b.WriteString("- p prune telemetry older than 30 days on observability\n")
	b.WriteString("- esc clears transient messages or cancels OAuth login\n")
	b.WriteString("- q quits\n")
	b.WriteString("\nPrivacy\n")
	b.WriteString("The TUI renders snapshot metadata and redacted display values only. It does not render prompts, completions, request bodies, response bodies, raw streams, tool arguments, tool results, provider payloads, provider request IDs, full local tokens, or full provider account IDs.\n")
}
