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
	svc := credentials.Service{Repo: store}
	created, err := svc.Create(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default(t.TempDir())
	handler := New(cfg, svc, store).Handler()

	unauth := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`not json`))
	unauthRec := httptest.NewRecorder()
	handler.ServeHTTP(unauthRec, unauth)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated request status=%d", unauthRec.Code)
	}

	body := []byte(`{"model":"deepseek/deepseek-v4-pro","messages":[{"role":"user","content":"check"}],"unknown":true}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+created.Token)
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
	svc := credentials.Service{Repo: store}
	created, err := svc.Create(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}

	handler := New(config.Default(t.TempDir()), svc, store).Handler()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+created.Token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("models status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDisabledTokenCannotAuthenticate(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "ilonasin.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := credentials.Service{Repo: store}
	created, err := svc.Create(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Disable(ctx, created.Metadata.ID); err != nil {
		t.Fatal(err)
	}

	handler := New(config.Default(t.TempDir()), svc, store).Handler()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+created.Token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("disabled token status=%d", rec.Code)
	}
}

func TestProviderCredentialSecretCannotAuthenticateLocalAPI(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "ilonasin.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	providerSecret := "iln_provider_secret_should_not_authenticate"
	res, err := store.DB.ExecContext(ctx, `
		INSERT INTO provider_credentials(provider_instance_id, kind, label)
		VALUES('deepseek', 'api_key', 'provider key')
	`)
	if err != nil {
		t.Fatal(err)
	}
	credentialID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.ExecContext(ctx, `
		INSERT INTO credential_secrets(credential_id, secret_kind, secret_material)
		VALUES(?, 'api_key', ?)
	`, credentialID, providerSecret); err != nil {
		t.Fatal(err)
	}

	handler := New(config.Default(t.TempDir()), credentials.Service{Repo: store}, store).Handler()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+providerSecret)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("provider secret authenticated as local token, status=%d", rec.Code)
	}
}
