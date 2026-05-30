package credentials

import "testing"

func TestRedactSecretDoesNotRevealShortSecrets(t *testing.T) {
	if got := RedactSecret("short"); got == "short" {
		t.Fatal("short secret was revealed")
	}
	if got := RedactSecret("123456789abcdef"); got != "12345678...redacted...cdef" {
		t.Fatalf("unexpected redaction %q", got)
	}
}
