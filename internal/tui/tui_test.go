package tui

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"ilonasin/internal/config"
	"ilonasin/internal/credentials"
)

func TestCheckDoesNotPrintGeneratedToken(t *testing.T) {
	svc := &fakeTokenManager{}
	if err := ExerciseTokenLifecycle(context.Background(), svc); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Check(config.Default(t.TempDir()), svc, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), svc.createdToken) {
		t.Fatal("check output leaked generated token")
	}
}

type fakeTokenManager struct {
	next         int64
	createdToken string
	rows         []credentials.LocalTokenMetadata
}

func (f *fakeTokenManager) Create(context.Context, string) (credentials.CreatedLocalToken, error) {
	if f.next == 0 {
		f.next = 1
	}
	token := "iln_fake_generated_token_abcdefghijklmnopqrstuvwxyz"
	f.createdToken = token
	meta := credentials.LocalTokenMetadata{
		ID:          f.next,
		Label:       "test",
		TokenPrefix: credentials.Prefix(token),
		TokenLast4:  credentials.Last4(token),
		CreatedAt:   time.Now().UTC(),
	}
	f.next++
	f.rows = append(f.rows, meta)
	return credentials.CreatedLocalToken{Token: token, Metadata: meta}, nil
}

func (f *fakeTokenManager) List(context.Context) ([]credentials.LocalTokenMetadata, error) {
	return append([]credentials.LocalTokenMetadata(nil), f.rows...), nil
}

func (f *fakeTokenManager) Disable(_ context.Context, id int64) error {
	now := time.Now().UTC()
	for i := range f.rows {
		if f.rows[i].ID == id {
			f.rows[i].Disabled = true
			f.rows[i].DisabledAt = &now
		}
	}
	return nil
}
