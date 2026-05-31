package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"ilonasin/internal/credentials"
	"ilonasin/internal/metadata"
	"ilonasin/internal/provider"
)

type migrationSmokeCase struct {
	name          string
	setup         func(context.Context, string) (map[int]migrationRow, error)
	wantVersions  []int
	wantSentinels bool
}

type migrationRow struct {
	name      string
	appliedAt string
}

// RunMigrationSmokeCheck exercises current migrations against historical
// schemas without using the selected user home.
func RunMigrationSmokeCheck(ctx context.Context) error {
	root, err := os.MkdirTemp("", "ilonasin-migration-smoke-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)

	currentVersions := currentMigrationVersions()
	cases := []migrationSmokeCase{
		{name: "fresh", setup: setupFreshMigrationSmoke, wantVersions: currentVersions},
		{name: "version1", setup: setupHistoricalMigrationSmoke(1), wantVersions: currentVersions, wantSentinels: true},
		{name: "version2", setup: setupHistoricalMigrationSmoke(2), wantVersions: currentVersions, wantSentinels: true},
		{name: "version3", setup: setupHistoricalMigrationSmoke(3), wantVersions: currentVersions, wantSentinels: true},
		{name: "drifted-version1", setup: setupDriftedVersion1MigrationSmoke, wantVersions: currentVersions, wantSentinels: true},
	}
	for _, tc := range cases {
		path := filepath.Join(root, tc.name+".sqlite")
		before, err := tc.setup(ctx, path)
		if err != nil {
			return fmt.Errorf("setup migration smoke %s: %w", tc.name, err)
		}
		store, err := Open(ctx, path)
		if err != nil {
			return fmt.Errorf("open migration smoke %s: %w", tc.name, err)
		}
		if err := assertMigrationSmokeState(ctx, store, tc, before); err != nil {
			_ = store.Close()
			return fmt.Errorf("migration smoke %s: %w", tc.name, err)
		}
		if err := exerciseMigratedStore(ctx, store, tc.name); err != nil {
			_ = store.Close()
			return fmt.Errorf("migration smoke %s behavior: %w", tc.name, err)
		}
		if err := store.Close(); err != nil {
			return fmt.Errorf("close migration smoke %s: %w", tc.name, err)
		}
		reopened, err := Open(ctx, path)
		if err != nil {
			return fmt.Errorf("reopen migration smoke %s: %w", tc.name, err)
		}
		if err := assertMigrationSmokeState(ctx, reopened, tc, before); err != nil {
			_ = reopened.Close()
			return fmt.Errorf("migration smoke %s after reopen: %w", tc.name, err)
		}
		if err := reopened.Close(); err != nil {
			return fmt.Errorf("close reopened migration smoke %s: %w", tc.name, err)
		}
	}
	return nil
}

func currentMigrationVersions() []int {
	out := make([]int, 0, len(migrations))
	for _, migration := range migrations {
		out = append(out, migration.version)
	}
	return out
}

func setupFreshMigrationSmoke(context.Context, string) (map[int]migrationRow, error) {
	return nil, nil
}

func setupHistoricalMigrationSmoke(version int) func(context.Context, string) (map[int]migrationRow, error) {
	return func(ctx context.Context, path string) (map[int]migrationRow, error) {
		db, err := openRawSmokeDB(path)
		if err != nil {
			return nil, err
		}
		defer db.Close()
		if err := applySmokeSQL(ctx, db, migration001); err != nil {
			return nil, err
		}
		before := map[int]migrationRow{}
		if err := insertMigrationSmokeRow(ctx, db, before, 1, "initial_schema"); err != nil {
			return nil, err
		}
		if version >= 2 {
			if err := applySmokeSQL(ctx, db, []string{
				`ALTER TABLE provider_credentials ADD COLUMN secret_prefix TEXT NOT NULL DEFAULT ''`,
				`ALTER TABLE provider_credentials ADD COLUMN secret_last4 TEXT NOT NULL DEFAULT ''`,
			}); err != nil {
				return nil, err
			}
			if err := insertMigrationSmokeRow(ctx, db, before, 2, "provider_credential_display_metadata"); err != nil {
				return nil, err
			}
		}
		if version >= 3 {
			if err := applySmokeSQL(ctx, db, []string{
				`ALTER TABLE provider_credentials ADD COLUMN fallback_group TEXT NOT NULL DEFAULT 'default'`,
				`CREATE TABLE IF NOT EXISTS credential_fallback_policies (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					provider_instance_id TEXT NOT NULL,
					group_label TEXT NOT NULL,
					enabled INTEGER NOT NULL DEFAULT 0,
					created_at TEXT NOT NULL,
					updated_at TEXT NOT NULL,
					UNIQUE(provider_instance_id, group_label)
				)`,
			}); err != nil {
				return nil, err
			}
			if err := insertMigrationSmokeRow(ctx, db, before, 3, "credential_fallback_policy"); err != nil {
				return nil, err
			}
		}
		if err := insertMigrationSmokeSentinel(ctx, db, "historical"); err != nil {
			return nil, err
		}
		return before, nil
	}
}

func setupDriftedVersion1MigrationSmoke(ctx context.Context, path string) (map[int]migrationRow, error) {
	db, err := openRawSmokeDB(path)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := applySmokeSQL(ctx, db, migration001); err != nil {
		return nil, err
	}
	before := map[int]migrationRow{}
	if err := insertMigrationSmokeRow(ctx, db, before, 1, "initial_schema"); err != nil {
		return nil, err
	}
	if err := applySmokeSQL(ctx, db, []string{
		`ALTER TABLE provider_credentials ADD COLUMN secret_prefix TEXT NOT NULL DEFAULT ''`,
	}); err != nil {
		return nil, err
	}
	if err := insertMigrationSmokeSentinel(ctx, db, "drifted"); err != nil {
		return nil, err
	}
	return before, nil
}

func openRawSmokeDB(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", "file:"+path+"?_foreign_keys=on&_busy_timeout=5000&_loc=UTC")
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func applySmokeSQL(ctx context.Context, db *sql.DB, stmts []string) error {
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func insertMigrationSmokeRow(ctx context.Context, db *sql.DB, rows map[int]migrationRow, version int, name string) error {
	row := migrationRow{name: name, appliedAt: fmt.Sprintf("2026-05-30T12:%02d:00Z", version)}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO migrations(version, name, applied_at)
		VALUES(?, ?, ?)
	`, version, row.name, row.appliedAt); err != nil {
		return err
	}
	rows[version] = row
	return nil
}

func insertMigrationSmokeSentinel(ctx context.Context, db *sql.DB, label string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO client_tokens(label, token_hash, token_prefix, token_last4, created_at)
		VALUES(?, ?, 'smoke', 'last', '2026-05-30T12:00:00Z')
	`, "migration-smoke-"+label, "migration-smoke-hash-"+label)
	return err
}

func assertMigrationSmokeState(ctx context.Context, store *Store, tc migrationSmokeCase, before map[int]migrationRow) error {
	if err := assertMigrationRows(ctx, store.DB, tc.wantVersions, before); err != nil {
		return err
	}
	if tc.wantSentinels {
		var count int
		if err := store.DB.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM client_tokens WHERE label LIKE 'migration-smoke-%'
		`).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("sentinel count=%d want=1", count)
		}
	}
	for _, check := range []columnCheck{
		{table: "provider_credentials", column: "secret_prefix", notNull: true, defaultValue: "''"},
		{table: "provider_credentials", column: "secret_last4", notNull: true, defaultValue: "''"},
		{table: "provider_credentials", column: "fallback_group", notNull: true, defaultValue: "'default'"},
		{table: "credential_fallback_policies", column: "credential_kind", notNull: true, defaultValue: "'api_key'"},
		{table: "oauth_tokens", column: "last_refresh_at"},
		{table: "oauth_tokens", column: "refresh_failure_class"},
		{table: "oauth_tokens", column: "refresh_failure_description", notNull: true, defaultValue: "''"},
		{table: "request_metadata", column: "retry_count", notNull: true, defaultValue: "0"},
		{table: "request_metadata", column: "fallback_count", notNull: true, defaultValue: "0"},
		{table: "request_metadata", column: "fallback_reason", notNull: true, defaultValue: "''"},
		{table: "request_metadata", column: "cache_write_tokens", notNull: true, defaultValue: "0"},
		{table: "request_metadata", column: "cost_microunits", notNull: true, defaultValue: "0"},
		{table: "request_metadata", column: "total_latency_ms", notNull: true, defaultValue: "0"},
		{table: "request_metadata", column: "time_to_first_token_ms", notNull: true, defaultValue: "0"},
		{table: "request_metadata", column: "output_tokens_per_second", notNull: true, defaultValue: "0"},
	} {
		if err := assertColumn(ctx, store.DB, check); err != nil {
			return err
		}
	}
	for _, table := range []string{"credential_fallback_policies", "model_cache", "request_metadata", "stream_metrics", "health_events", "fallback_events", "quota_events"} {
		if err := assertTableExists(ctx, store.DB, table); err != nil {
			return err
		}
	}
	for _, check := range []foreignKeyCheck{
		{table: "credential_secrets", from: "credential_id", targetTable: "provider_credentials", onDelete: "CASCADE"},
		{table: "oauth_tokens", from: "credential_id", targetTable: "provider_credentials", onDelete: "CASCADE"},
		{table: "oauth_tokens", from: "access_token_secret_id", targetTable: "credential_secrets", onDelete: "SET NULL"},
		{table: "oauth_tokens", from: "refresh_token_secret_id", targetTable: "credential_secrets", onDelete: "SET NULL"},
		{table: "provider_accounts", from: "credential_id", targetTable: "provider_credentials", onDelete: "SET NULL"},
		{table: "request_metadata", from: "client_token_id", targetTable: "client_tokens", onDelete: "SET NULL"},
		{table: "request_metadata", from: "credential_id", targetTable: "provider_credentials", onDelete: "SET NULL"},
		{table: "stream_metrics", from: "request_metadata_id", targetTable: "request_metadata", onDelete: "CASCADE"},
		{table: "health_events", from: "credential_id", targetTable: "provider_credentials", onDelete: "SET NULL"},
		{table: "fallback_events", from: "request_metadata_id", targetTable: "request_metadata", onDelete: "CASCADE"},
		{table: "fallback_events", from: "from_credential_id", targetTable: "provider_credentials", onDelete: "SET NULL"},
		{table: "fallback_events", from: "to_credential_id", targetTable: "provider_credentials", onDelete: "SET NULL"},
		{table: "quota_events", from: "request_metadata_id", targetTable: "request_metadata", onDelete: "CASCADE"},
		{table: "quota_events", from: "credential_id", targetTable: "provider_credentials", onDelete: "SET NULL"},
	} {
		if err := assertForeignKey(ctx, store.DB, check); err != nil {
			return err
		}
	}
	return nil
}

func assertMigrationRows(ctx context.Context, db *sql.DB, wantVersions []int, before map[int]migrationRow) error {
	for _, version := range wantVersions {
		var name, appliedAt string
		if err := db.QueryRowContext(ctx, `
			SELECT name, applied_at FROM migrations WHERE version = ?
		`, version).Scan(&name, &appliedAt); err != nil {
			return fmt.Errorf("migration version %d: %w", version, err)
		}
		if row, ok := before[version]; ok && (row.name != name || row.appliedAt != appliedAt) {
			return fmt.Errorf("migration version %d changed from %q/%q to %q/%q", version, row.name, row.appliedAt, name, appliedAt)
		}
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM migrations`).Scan(&count); err != nil {
		return err
	}
	if count != len(wantVersions) {
		return fmt.Errorf("migration row count=%d want=%d", count, len(wantVersions))
	}
	return nil
}

type columnCheck struct {
	table        string
	column       string
	notNull      bool
	defaultValue string
}

func assertColumn(ctx context.Context, db *sql.DB, check columnCheck) error {
	rows, err := db.QueryContext(ctx, "PRAGMA table_info("+check.table+")")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name != check.column {
			continue
		}
		if check.notNull && notNull != 1 {
			return fmt.Errorf("%s.%s not_null=%d want=1", check.table, check.column, notNull)
		}
		if check.defaultValue != "" && (!defaultValue.Valid || defaultValue.String != check.defaultValue) {
			return fmt.Errorf("%s.%s default=%q want=%q", check.table, check.column, defaultValue.String, check.defaultValue)
		}
		return nil
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return fmt.Errorf("missing column %s.%s", check.table, check.column)
}

func assertTableExists(ctx context.Context, db *sql.DB, table string) error {
	var name string
	err := db.QueryRowContext(ctx, `
		SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?
	`, table).Scan(&name)
	if err != nil {
		return fmt.Errorf("missing table %s: %w", table, err)
	}
	return nil
}

type foreignKeyCheck struct {
	table       string
	from        string
	targetTable string
	onDelete    string
}

func assertForeignKey(ctx context.Context, db *sql.DB, check foreignKeyCheck) error {
	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_list("+check.table+")")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, seq int
		var targetTable, from, to, onUpdate, onDelete, match string
		if err := rows.Scan(&id, &seq, &targetTable, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			return err
		}
		if from == check.from && targetTable == check.targetTable && onDelete == check.onDelete {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return fmt.Errorf("missing foreign key %s.%s -> %s ON DELETE %s", check.table, check.from, check.targetTable, check.onDelete)
}

func exerciseMigratedStore(ctx context.Context, store *Store, suffix string) error {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	api, err := store.InsertAPIKeyCredential(ctx, credentials.NewUpstreamCredential{
		ProviderInstanceID: "deepseek",
		Kind:               "api_key",
		Label:              "migration smoke api " + suffix,
		SecretPrefix:       "sk-mig",
		SecretLast4:        "last",
		FallbackGroup:      credentials.DefaultFallbackGroup,
		CreatedAt:          now,
	}, "sk-migration-smoke-secret-"+suffix)
	if err != nil {
		return err
	}
	if _, err := store.InsertAPIKeyCredential(ctx, credentials.NewUpstreamCredential{
		ProviderInstanceID: "deepseek",
		Kind:               "api_key",
		Label:              api.Label,
		SecretPrefix:       "sk-dup",
		SecretLast4:        "last",
		FallbackGroup:      credentials.DefaultFallbackGroup,
		CreatedAt:          now,
	}, "sk-migration-smoke-duplicate-"+suffix); !errors.Is(err, credentials.ErrDuplicateCredential) {
		return fmt.Errorf("duplicate api credential err=%v want %v", err, credentials.ErrDuplicateCredential)
	}
	listed, err := store.ListUpstreamCredentials(ctx)
	if err != nil {
		return err
	}
	if len(listed) == 0 {
		return fmt.Errorf("api credential list empty")
	}
	resolved, err := store.ResolveAPIKeyCredential(ctx, "deepseek")
	if err != nil {
		return err
	}
	if resolved.APIKey != "sk-migration-smoke-secret-"+suffix {
		return fmt.Errorf("resolved unexpected api key")
	}

	expires := now.Add(time.Hour)
	oauth, err := store.InsertOAuthCredential(ctx, credentials.NewOAuthCredential{
		ProviderInstanceID:  "codex",
		Label:               "migration smoke oauth " + suffix,
		AccountHash:         "migration-smoke-account-hash-" + suffix,
		AccountDisplayLabel: "Migration Smoke",
		PlanLabel:           "team",
		Scopes:              "openid profile email",
		ExpiresAt:           &expires,
		CreatedAt:           now,
	}, "oauth-migration-smoke-access-"+suffix, "oauth-migration-smoke-refresh-"+suffix)
	if err != nil {
		return err
	}
	if _, err := store.ListOAuthCredentials(ctx); err != nil {
		return err
	}
	if _, err := store.ListProviderAccounts(ctx); err != nil {
		return err
	}
	if err := store.UpdateOAuthTokens(ctx, oauth.ID, credentials.OAuthTokenUpdate{
		AccessToken:  "oauth-migration-smoke-access-new-" + suffix,
		RefreshToken: "oauth-migration-smoke-refresh-new-" + suffix,
		ExpiresAt:    &expires,
		RefreshedAt:  now,
	}); err != nil {
		return err
	}
	if err := store.MarkOAuthRefreshFailure(ctx, oauth.ID, "invalid_grant", "refresh failed", now); err != nil {
		return err
	}

	if err := store.ReplaceModelCache(ctx, "deepseek", []provider.ModelMetadata{{
		ProviderInstanceID: "deepseek",
		ModelID:            "deepseek-chat",
		DisplayName:        "DeepSeek Chat",
		CapabilityFlags:    "chat,stream",
		ContextLength:      128000,
		UpdatedAt:          now,
	}}); err != nil {
		return err
	}
	if _, err := store.ListModelCache(ctx); err != nil {
		return err
	}

	requestID, err := store.RecordRequestMetadata(ctx, metadata.Request{
		StartedAt:                 now,
		CredentialID:              api.ID,
		RequestedProviderInstance: "deepseek",
		RequestedModel:            "deepseek-chat",
		ResolvedProviderInstance:  "deepseek",
		ResolvedModel:             "deepseek-chat",
		HTTPStatus:                200,
		RetryCount:                1,
		FallbackCount:             1,
		PromptTokens:              1,
		CompletionTokens:          2,
		TotalTokens:               3,
		ReasoningTokens:           0,
		CacheHitTokens:            0,
		CacheWriteTokens:          1,
		CostMicrounits:            2,
		TotalLatencyMS:            10,
		TimeToFirstTokenMS:        5,
		OutputTokensPerSecond:     2,
	})
	if err != nil {
		return err
	}
	if err := store.RecordStreamMetrics(ctx, metadata.Stream{
		RequestMetadataID:     requestID,
		TimeToFirstTokenMS:    5,
		OutputTokensPerSecond: 2,
		CompletionStatus:      "completed",
		ChunkCount:            2,
	}); err != nil {
		return err
	}
	resolvedRequestID, err := store.RecordRequestMetadata(ctx, metadata.Request{
		StartedAt:                 now.Add(time.Second),
		CredentialID:              api.ID,
		RequestedProviderInstance: "openrouter",
		RequestedModel:            "openrouter/auto",
		ResolvedProviderInstance:  "openrouter",
		ResolvedModel:             "deepseek/deepseek-v4-flash:free",
		HTTPStatus:                200,
		PromptTokens:              1,
		CompletionTokens:          1,
		TotalTokens:               2,
		TotalLatencyMS:            11,
	})
	if err != nil {
		return err
	}
	recent, err := store.RecentRequests(ctx, 1)
	if err != nil {
		return err
	}
	if len(recent) != 1 || recent[0].ID != resolvedRequestID ||
		recent[0].RequestedProviderID != "openrouter" ||
		recent[0].RequestedModelID != "openrouter/auto" ||
		recent[0].ResolvedProviderID != "openrouter" ||
		recent[0].ResolvedModelID != "deepseek/deepseek-v4-flash:free" ||
		recent[0].ProviderInstanceID != "openrouter" ||
		recent[0].ModelID != "deepseek/deepseek-v4-flash:free" {
		return fmt.Errorf("recent request resolved metadata mismatch")
	}
	retryAfter := now.Add(2 * time.Minute)
	if err := store.RecordHealthEvent(ctx, metadata.HealthEvent{
		OccurredAt:         now,
		ProviderInstanceID: "deepseek",
		CredentialID:       api.ID,
		ModelID:            "deepseek-chat",
		EventClass:         "upstream_failure",
		HTTPStatus:         429,
		ErrorClass:         "upstream_http_error",
		RetryAfter:         &retryAfter,
	}); err != nil {
		return err
	}
	healthRows, err := store.LatestHealth(ctx)
	if err != nil {
		return err
	}
	if len(healthRows) == 0 || healthRows[0].RetryAfter == nil || !healthRows[0].RetryAfter.Equal(retryAfter.UTC()) {
		return fmt.Errorf("retry-after health metadata missing")
	}
	if err := store.RecordFallbackEvent(ctx, metadata.FallbackEvent{
		RequestMetadataID:  requestID,
		OccurredAt:         now,
		ProviderInstanceID: "deepseek",
		ModelID:            "deepseek-chat",
		FromCredentialID:   api.ID,
		ToCredentialID:     api.ID,
		Reason:             "retryable_status",
		AllowedByPolicy:    true,
	}); err != nil {
		return err
	}
	if err := store.SetFallbackGroupEnabled(ctx, "deepseek", credentials.CredentialKindAPIKey, credentials.DefaultFallbackGroup, true, now); err != nil {
		return err
	}
	policies, err := store.ListFallbackPolicies(ctx)
	if err != nil {
		return err
	}
	if len(policies) == 0 {
		return fmt.Errorf("fallback policy list empty")
	}

	cascade, err := store.InsertAPIKeyCredential(ctx, credentials.NewUpstreamCredential{
		ProviderInstanceID: "openrouter",
		Kind:               "api_key",
		Label:              "migration smoke cascade " + suffix,
		SecretPrefix:       "sk-cas",
		SecretLast4:        "last",
		FallbackGroup:      credentials.DefaultFallbackGroup,
		CreatedAt:          now,
	}, "sk-migration-smoke-cascade-"+suffix)
	if err != nil {
		return err
	}
	if _, err := store.DB.ExecContext(ctx, `DELETE FROM provider_credentials WHERE id = ?`, cascade.ID); err != nil {
		return err
	}
	var secretCount int
	if err := store.DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM credential_secrets WHERE credential_id = ?
	`, cascade.ID).Scan(&secretCount); err != nil {
		return err
	}
	if secretCount != 0 {
		return fmt.Errorf("credential secret cascade count=%d want=0", secretCount)
	}
	if err := assertMigrationSmokeSecrets(ctx, store.DB); err != nil {
		return err
	}
	return nil
}

func assertMigrationSmokeSecrets(ctx context.Context, db *sql.DB) error {
	tables := []struct {
		name    string
		columns []string
	}{
		{"client_tokens", []string{"label", "token_hash", "token_prefix", "token_last4"}},
		{"provider_credentials", []string{"provider_instance_id", "kind", "label", "secret_prefix", "secret_last4", "fallback_group"}},
		{"oauth_tokens", []string{"scopes", "refresh_failure_class", "refresh_failure_description"}},
		{"provider_accounts", []string{"provider_instance_id", "account_hash", "display_label", "plan_label"}},
		{"model_cache", []string{"provider_instance_id", "model_id", "display_name", "capability_flags"}},
		{"request_metadata", []string{"requested_provider_instance", "requested_model", "resolved_provider_instance", "resolved_model", "error_class", "fallback_reason"}},
		{"health_events", []string{"provider_instance_id", "model_id", "event_class", "normalized_error_class", "refresh_failure_class"}},
		{"fallback_events", []string{"provider_instance_id", "model_id", "reason"}},
		{"quota_events", []string{"provider_instance_id", "model_id", "source", "error_class"}},
		{"credential_fallback_policies", []string{"provider_instance_id", "credential_kind", "group_label"}},
	}
	for _, table := range tables {
		for _, column := range table.columns {
			query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s LIKE '%%migration-smoke-secret%%' OR %s LIKE '%%migration-smoke-access%%' OR %s LIKE '%%migration-smoke-refresh%%'", table.name, column, column, column)
			var count int
			if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil {
				return err
			}
			if count != 0 {
				return fmt.Errorf("secret marker leaked into %s.%s", table.name, column)
			}
		}
	}
	var secretCount int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM credential_secrets
		WHERE secret_material LIKE '%migration-smoke-secret%'
			OR secret_material LIKE '%migration-smoke-access%'
			OR secret_material LIKE '%migration-smoke-refresh%'
	`).Scan(&secretCount); err != nil {
		return err
	}
	if secretCount == 0 {
		return fmt.Errorf("secret marker missing from credential_secrets")
	}
	var leaked int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM credential_secrets
		WHERE secret_material LIKE '%raw-provider-payload%'
			OR secret_material LIKE '%prompt marker%'
			OR secret_material LIKE '%request body%'
			OR secret_material LIKE '%response body%'
	`).Scan(&leaked); err != nil {
		return err
	}
	if leaked != 0 {
		return fmt.Errorf("unsafe payload marker found in credential_secrets")
	}
	return nil
}
