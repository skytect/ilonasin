package cli

import (
	"bytes"
	"testing"
)

func TestRunRejectsUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"unknown"}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit code=%d", code)
	}
	if stderr.Len() == 0 {
		t.Fatal("expected usage on stderr")
	}
}
