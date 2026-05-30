package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"ilonasin/internal/config"
	"ilonasin/internal/credentials"
	"ilonasin/internal/storage/sqlite"
)

func TestAuthBeforeBodyParsingAndStrictValidation(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "ilonasin.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	token := "iln_test_high_entropy_token_value"
	if err := store.InsertClientToken(ctx, "test", token); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default(t.TempDir())
	handler := New(cfg, credentials.Authenticator{Store: store}, store).Handler()

	unauth := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`not json`))
	unauthRec := httptest.NewRecorder()
	handler.ServeHTTP(unauthRec, unauth)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated request status=%d", unauthRec.Code)
	}

	body := []byte(`{"model":"deepseek/deepseek-v4-pro","messages":[{"role":"user","content":"check"}],"unknown":true}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("strict validation status=%d body=%s", rec.Code, rec.Body.String())
	}

	var count int
	if err := store.DB.QueryRowContext(ctx, "SELECT count(*) FROM request_metadata").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("invalid request should not persist request metadata, got %d rows", count)
	}
}

func TestAuthenticatedModels(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "ilonasin.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	token := "iln_test_high_entropy_token_value"
	if err := store.InsertClientToken(ctx, "test", token); err != nil {
		t.Fatal(err)
	}

	handler := New(config.Default(t.TempDir()), credentials.Authenticator{Store: store}, store).Handler()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("models status=%d body=%s", rec.Code, rec.Body.String())
	}
}
