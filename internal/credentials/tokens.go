package credentials

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
)

type ClientTokenRecord struct {
	ID          int64
	Label       string
	TokenHash   string
	TokenPrefix string
	TokenLast4  string
	Disabled    bool
}

type ClientTokenStore interface {
	FindClientTokenByHash(ctx context.Context, hash string) (ClientTokenRecord, error)
}

var ErrUnauthorized = errors.New("unauthorized")

type Authenticator struct {
	Store ClientTokenStore
}

func GenerateToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "iln_" + base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func Prefix(token string) string {
	if len(token) <= 8 {
		return token
	}
	return token[:8]
}

func Last4(token string) string {
	if len(token) <= 4 {
		return token
	}
	return token[len(token)-4:]
}

func (a Authenticator) VerifyBearer(ctx context.Context, authorization string) (ClientTokenRecord, error) {
	const prefix = "Bearer "
	if !strings.HasPrefix(authorization, prefix) {
		return ClientTokenRecord{}, ErrUnauthorized
	}
	token := strings.TrimSpace(strings.TrimPrefix(authorization, prefix))
	if token == "" {
		return ClientTokenRecord{}, ErrUnauthorized
	}
	hash := HashToken(token)
	rec, err := a.Store.FindClientTokenByHash(ctx, hash)
	if err != nil || rec.Disabled {
		return ClientTokenRecord{}, ErrUnauthorized
	}
	if subtle.ConstantTimeCompare([]byte(rec.TokenHash), []byte(hash)) != 1 {
		return ClientTokenRecord{}, ErrUnauthorized
	}
	return rec, nil
}

func RedactSecret(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "redacted"
	}
	return Prefix(value) + "...redacted..." + Last4(value)
}
