package app

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"ilonasin/internal/storage/sqlite"
)

func TestServeCheckDoesNotSeedSelectedHomeDatabase(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("ILONASIN_HOME", homeDir)
	if err := ServeCheck(Options{Stdout: io.Discard, Stderr: io.Discard}); err != nil {
		t.Fatal(err)
	}

	store, err := sqlite.Open(context.Background(), filepath.Join(homeDir, "ilonasin.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var count int
	if err := store.DB.QueryRowContext(context.Background(), "SELECT count(*) FROM client_tokens").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("serve check left %d client token rows in selected home database", count)
	}
}
