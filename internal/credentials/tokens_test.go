package credentials

import (
	"context"
	"testing"
	"time"
)

func TestRedactSecretDoesNotRevealShortSecrets(t *testing.T) {
	if got := RedactSecret("short"); got == "short" {
		t.Fatal("short secret was revealed")
	}
	if got := RedactSecret("123456789abcdef"); got != "12345678...redacted...cdef" {
		t.Fatalf("unexpected redaction %q", got)
	}
}

func TestServiceCreateVerifyDisable(t *testing.T) {
	repo := newMemoryRepo()
	now := time.Date(2026, 5, 30, 1, 2, 3, 0, time.UTC)
	svc := Service{Repo: repo, Now: func() time.Time { return now }}
	created, err := svc.Create(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Token) < len("iln_")+43 {
		t.Fatalf("token too short: %d", len(created.Token))
	}
	if got := repo.rows[created.Metadata.ID].hash; got == created.Token {
		t.Fatal("repo received plaintext token")
	}
	if _, err := svc.VerifyBearer(context.Background(), "Bearer "+created.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.VerifyBearer(context.Background(), "Bearer sk-provider"); err != ErrUnauthorized {
		t.Fatalf("provider-looking token err=%v", err)
	}
	if err := svc.Disable(context.Background(), created.Metadata.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Disable(context.Background(), created.Metadata.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.VerifyBearer(context.Background(), "Bearer "+created.Token); err != ErrUnauthorized {
		t.Fatalf("disabled token err=%v", err)
	}
	list, err := svc.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || !list[0].Disabled || list[0].DisabledAt == nil {
		t.Fatalf("unexpected list %#v", list)
	}
}

type memoryRepo struct {
	next int64
	rows map[int64]memoryToken
}

type memoryToken struct {
	meta LocalTokenMetadata
	hash string
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{next: 1, rows: map[int64]memoryToken{}}
}

func (r *memoryRepo) InsertLocalToken(_ context.Context, meta NewLocalTokenMetadata) (LocalTokenMetadata, error) {
	id := r.next
	r.next++
	out := LocalTokenMetadata{
		ID:          id,
		Label:       meta.Label,
		TokenPrefix: meta.TokenPrefix,
		TokenLast4:  meta.TokenLast4,
		CreatedAt:   meta.CreatedAt,
	}
	r.rows[id] = memoryToken{meta: out, hash: meta.TokenHash}
	return out, nil
}

func (r *memoryRepo) ListLocalTokens(context.Context) ([]LocalTokenMetadata, error) {
	out := make([]LocalTokenMetadata, 0, len(r.rows))
	for id := int64(1); id < r.next; id++ {
		if row, ok := r.rows[id]; ok {
			out = append(out, row.meta)
		}
	}
	return out, nil
}

func (r *memoryRepo) DisableLocalToken(_ context.Context, id int64, disabledAt time.Time) error {
	row, ok := r.rows[id]
	if !ok {
		return ErrUnauthorized
	}
	if row.meta.DisabledAt == nil {
		row.meta.DisabledAt = &disabledAt
		row.meta.Disabled = true
		r.rows[id] = row
	}
	return nil
}

func (r *memoryRepo) FindLocalTokenByHash(_ context.Context, hash string) (LocalTokenAuthRecord, error) {
	for _, row := range r.rows {
		if row.hash == hash {
			return LocalTokenAuthRecord{
				ID:        row.meta.ID,
				Label:     row.meta.Label,
				TokenHash: row.hash,
				Disabled:  row.meta.Disabled,
			}, nil
		}
	}
	return LocalTokenAuthRecord{}, ErrUnauthorized
}
