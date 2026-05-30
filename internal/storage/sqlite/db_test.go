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
	token := "iln_test_high_entropy_token_value"
	if err := store.InsertClientToken(ctx, "test", token); err != nil {
		t.Fatal(err)
	}
	rec, err := store.FindClientTokenByHash(ctx, credentials.HashToken(token))
	if err != nil {
		t.Fatal(err)
	}
	if rec.TokenHash == token || rec.TokenPrefix == token {
		t.Fatal("raw token was stored in token hash or prefix")
	}
	if rec.TokenLast4 != "alue" {
		t.Fatalf("unexpected last4 %q", rec.TokenLast4)
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
