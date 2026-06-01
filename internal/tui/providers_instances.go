package tui

import (
	"fmt"
	"strings"
)

func (m Model) writeProviderInstances(b *strings.Builder) {
	b.WriteString("\nProvider instances\n")
	for _, instance := range m.providers {
		apiKey := "api-key disabled"
		if instance.APIKey {
			apiKey = "api-key"
		}
		oauth := "oauth disabled"
		if instance.OAuth {
			oauth = "oauth"
		}
		fmt.Fprintf(b, "- %s %s %s %s %s\n", instance.ID, instance.Type, instance.BaseURL, apiKey, oauth)
	}
}
