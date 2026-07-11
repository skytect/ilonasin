package provider

import (
	"strings"
	"testing"
)

func TestValidateCodexReasoningAcceptsMaxEffort(t *testing.T) {
	err := validateCodexOptions(map[string]any{
		"reasoning": map[string]any{"effort": "max"},
	})
	if err != nil {
		t.Fatalf("expected max effort to be accepted, got %v", err)
	}
}

func TestValidateCodexReasoningRejectsUnsupportedUltraEffort(t *testing.T) {
	err := validateCodexOptions(map[string]any{
		"reasoning": map[string]any{"effort": "ultra"},
	})
	if err == nil {
		t.Fatal("expected ultra effort to be rejected")
	}
	if !strings.Contains(err.Error(), "reasoning.effort is unsupported") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateCodexReasoningRejectsProMode(t *testing.T) {
	err := validateCodexOptions(map[string]any{
		"reasoning": map[string]any{"mode": "pro"},
	})
	if err == nil {
		t.Fatal("expected pro mode to be rejected")
	}
	if !strings.Contains(err.Error(), "reasoning contains an unsupported field") {
		t.Fatalf("unexpected error: %v", err)
	}
}
