package home

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandPathUsesUserHomeForTilde(t *testing.T) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	got := ExpandPath("~/.ilonasin/ilonasin.sqlite", "/tmp/ilonasin-home")
	want := filepath.Join(userHome, ".ilonasin", "ilonasin.sqlite")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
