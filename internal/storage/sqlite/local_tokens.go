package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"ilonasin/internal/credentials"
)

func (s *Store) InsertLocalToken(ctx context.Context, meta credentials.NewLocalTokenMetadata) (credentials.LocalTokenMetadata, error) {
	res, err := s.DB.ExecContext(ctx, `
		INSERT INTO client_tokens(label, token_hash, token_prefix, token_last4, created_at)
		VALUES(?, ?, ?, ?, ?)
	`, meta.Label, meta.TokenHash, meta.TokenPrefix, meta.TokenLast4, meta.CreatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return credentials.LocalTokenMetadata{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return credentials.LocalTokenMetadata{}, err
	}
	return credentials.LocalTokenMetadata{
		ID:          id,
		Label:       meta.Label,
		TokenPrefix: meta.TokenPrefix,
		TokenLast4:  meta.TokenLast4,
		CreatedAt:   meta.CreatedAt.UTC(),
	}, nil
}

func (s *Store) ListLocalTokens(ctx context.Context) ([]credentials.LocalTokenMetadata, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, label, token_prefix, token_last4, created_at, disabled_at
		FROM client_tokens
		ORDER BY id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []credentials.LocalTokenMetadata
	for rows.Next() {
		meta, err := scanLocalTokenMetadata(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, meta)
	}
	return out, rows.Err()
}

func (s *Store) DisableLocalToken(ctx context.Context, id int64, disabledAt time.Time) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE client_tokens
		SET disabled_at = COALESCE(disabled_at, ?)
		WHERE id = ?
	`, disabledAt.UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("client token not found")
	}
	return nil
}

func (s *Store) FindLocalTokenByHash(ctx context.Context, hash string) (credentials.LocalTokenAuthRecord, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, label, token_hash, token_prefix, token_last4, disabled_at IS NOT NULL
		FROM client_tokens
		WHERE token_hash = ?
	`, hash)
	var rec credentials.LocalTokenAuthRecord
	var unusedPrefix, unusedLast4 string
	if err := row.Scan(&rec.ID, &rec.Label, &rec.TokenHash, &unusedPrefix, &unusedLast4, &rec.Disabled); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return credentials.LocalTokenAuthRecord{}, fmt.Errorf("client token not found")
		}
		return credentials.LocalTokenAuthRecord{}, err
	}
	return rec, nil
}

func scanLocalTokenMetadata(row localTokenScanner) (credentials.LocalTokenMetadata, error) {
	var meta credentials.LocalTokenMetadata
	var created string
	var disabled sql.NullString
	if err := row.Scan(&meta.ID, &meta.Label, &meta.TokenPrefix, &meta.TokenLast4, &created, &disabled); err != nil {
		return credentials.LocalTokenMetadata{}, err
	}
	createdAt, err := parseSQLiteTime(created)
	if err != nil {
		return credentials.LocalTokenMetadata{}, err
	}
	meta.CreatedAt = createdAt
	if disabled.Valid {
		disabledAt, err := parseSQLiteTime(disabled.String)
		if err != nil {
			return credentials.LocalTokenMetadata{}, err
		}
		meta.DisabledAt = &disabledAt
		meta.Disabled = true
	}
	return meta, nil
}
