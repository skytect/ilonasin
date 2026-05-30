package sqlite

var migration001 = []string{
	`CREATE TABLE IF NOT EXISTS migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS client_tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		label TEXT NOT NULL,
		token_hash TEXT NOT NULL UNIQUE,
		token_prefix TEXT NOT NULL,
		token_last4 TEXT NOT NULL,
		created_at TEXT NOT NULL,
		disabled_at TEXT
	)`,
	`CREATE TABLE IF NOT EXISTS provider_credentials (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		provider_instance_id TEXT NOT NULL,
		kind TEXT NOT NULL,
		label TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		disabled_at TEXT,
		UNIQUE(provider_instance_id, label)
	)`,
	`CREATE TABLE IF NOT EXISTS credential_secrets (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		credential_id INTEGER NOT NULL REFERENCES provider_credentials(id) ON DELETE CASCADE,
		secret_kind TEXT NOT NULL,
		secret_material TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(credential_id, secret_kind)
	)`,
	`CREATE TABLE IF NOT EXISTS oauth_tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		credential_id INTEGER NOT NULL REFERENCES provider_credentials(id) ON DELETE CASCADE,
		access_token_secret_id INTEGER REFERENCES credential_secrets(id) ON DELETE SET NULL,
		refresh_token_secret_id INTEGER REFERENCES credential_secrets(id) ON DELETE SET NULL,
		expires_at TEXT,
		scopes TEXT NOT NULL DEFAULT '',
		last_refresh_at TEXT,
		refresh_failure_class TEXT,
		UNIQUE(credential_id)
	)`,
	`CREATE TABLE IF NOT EXISTS provider_accounts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		provider_instance_id TEXT NOT NULL,
		credential_id INTEGER REFERENCES provider_credentials(id) ON DELETE SET NULL,
		account_hash TEXT NOT NULL,
		display_label TEXT NOT NULL DEFAULT '',
		plan_label TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(provider_instance_id, account_hash)
	)`,
	`CREATE TABLE IF NOT EXISTS model_cache (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		provider_instance_id TEXT NOT NULL,
		model_id TEXT NOT NULL,
		display_name TEXT NOT NULL DEFAULT '',
		capability_flags TEXT NOT NULL DEFAULT '',
		context_length INTEGER,
		updated_at TEXT NOT NULL,
		UNIQUE(provider_instance_id, model_id)
	)`,
	`CREATE TABLE IF NOT EXISTS request_metadata (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		started_at TEXT NOT NULL,
		client_token_id INTEGER REFERENCES client_tokens(id) ON DELETE SET NULL,
		credential_id INTEGER REFERENCES provider_credentials(id) ON DELETE SET NULL,
		requested_provider_instance TEXT NOT NULL,
		requested_model TEXT NOT NULL,
		resolved_provider_instance TEXT NOT NULL,
		resolved_model TEXT NOT NULL,
		http_status INTEGER NOT NULL,
		error_class TEXT NOT NULL DEFAULT '',
		retry_count INTEGER NOT NULL DEFAULT 0,
		fallback_count INTEGER NOT NULL DEFAULT 0,
		fallback_reason TEXT NOT NULL DEFAULT '',
		prompt_tokens INTEGER NOT NULL DEFAULT 0,
		completion_tokens INTEGER NOT NULL DEFAULT 0,
		total_tokens INTEGER NOT NULL DEFAULT 0,
		reasoning_tokens INTEGER NOT NULL DEFAULT 0,
		cache_hit_tokens INTEGER NOT NULL DEFAULT 0,
		cache_write_tokens INTEGER NOT NULL DEFAULT 0,
		cost_microunits INTEGER NOT NULL DEFAULT 0,
		total_latency_ms INTEGER NOT NULL DEFAULT 0,
		time_to_first_token_ms INTEGER NOT NULL DEFAULT 0,
		output_tokens_per_second REAL NOT NULL DEFAULT 0
	)`,
	`CREATE TABLE IF NOT EXISTS stream_metrics (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		request_metadata_id INTEGER NOT NULL REFERENCES request_metadata(id) ON DELETE CASCADE,
		time_to_first_token_ms INTEGER NOT NULL DEFAULT 0,
		output_tokens_per_second REAL NOT NULL DEFAULT 0,
		completion_status TEXT NOT NULL,
		chunk_count INTEGER NOT NULL DEFAULT 0
	)`,
	`CREATE TABLE IF NOT EXISTS health_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		occurred_at TEXT NOT NULL,
		provider_instance_id TEXT NOT NULL,
		credential_id INTEGER REFERENCES provider_credentials(id) ON DELETE SET NULL,
		model_id TEXT NOT NULL DEFAULT '',
		event_class TEXT NOT NULL,
		http_status INTEGER,
		normalized_error_class TEXT NOT NULL DEFAULT '',
		consecutive_failure_count INTEGER NOT NULL DEFAULT 0,
		retry_after TEXT,
		token_expires_at TEXT,
		refresh_failure_class TEXT NOT NULL DEFAULT ''
	)`,
	`CREATE TABLE IF NOT EXISTS fallback_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		request_metadata_id INTEGER REFERENCES request_metadata(id) ON DELETE CASCADE,
		occurred_at TEXT NOT NULL,
		provider_instance_id TEXT NOT NULL,
		model_id TEXT NOT NULL,
		from_credential_id INTEGER REFERENCES provider_credentials(id) ON DELETE SET NULL,
		to_credential_id INTEGER REFERENCES provider_credentials(id) ON DELETE SET NULL,
		reason TEXT NOT NULL,
		allowed_by_policy INTEGER NOT NULL
	)`,
}

type migration struct {
	version int
	name    string
	stmts   []string
}

var migrations = []migration{
	{version: 1, name: "initial_schema", stmts: migration001},
	{version: 2, name: "provider_credential_display_metadata", stmts: []string{
		`ALTER TABLE provider_credentials ADD COLUMN secret_prefix TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE provider_credentials ADD COLUMN secret_last4 TEXT NOT NULL DEFAULT ''`,
	}},
}
