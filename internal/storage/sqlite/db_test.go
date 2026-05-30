package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"ilonasin/internal/credentials"
)

func TestMigrateIdempotentAndTokenHashLookup(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "ilonasin.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	svc := credentials.Service{Repo: store}
	created, err := svc.Create(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	rec, err := store.FindLocalTokenByHash(ctx, credentials.HashToken(created.Token))
	if err != nil {
		t.Fatal(err)
	}
	if rec.TokenHash == created.Token {
		t.Fatal("raw token was stored in token hash or prefix")
	}
	list, err := store.ListLocalTokens(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].TokenLast4 != credentials.Last4(created.Token) {
		t.Fatalf("unexpected list %#v", list)
	}
}

func TestTelemetryTablesDoNotExposeRawPayloadColumns(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "ilonasin.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	for _, table := range []string{"request_metadata", "stream_metrics", "health_events", "fallback_events"} {
		t.Run(table, func(t *testing.T) {
			rows, err := store.DB.QueryContext(ctx, "PRAGMA table_info("+table+")")
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			for rows.Next() {
				var cid int
				var name, typ string
				var notNull int
				var dflt sql.NullString
				var pk int
				if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
					t.Fatal(err)
				}
				lower := strings.ToLower(name)
				for _, forbidden := range []string{"body", "payload", "prompt_text", "completion_text", "raw", "sse", "cookie", "bearer", "account_id", "request_id", "generation_id"} {
					if strings.Contains(lower, forbidden) {
						t.Fatalf("telemetry table %s has forbidden column %s", table, name)
					}
				}
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestClientTokensTableHasNoPlaintextTokenColumn(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "ilonasin.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	rows, err := store.DB.QueryContext(ctx, "PRAGMA table_info(client_tokens)")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		if name == "token" || name == "plaintext_token" || name == "bearer_token" {
			t.Fatalf("client_tokens has plaintext column %s", name)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}
