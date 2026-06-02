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
	"time"
)

type VerifiedLocalToken struct {
	ID    int64
	Label string
}

type CreatedLocalToken struct {
	Token    string
	Metadata LocalTokenMetadata
}

type LocalTokenMetadata struct {
	ID          int64
	Label       string
	TokenPrefix string
	TokenLast4  string
	CreatedAt   time.Time
	DisabledAt  *time.Time
	Disabled    bool
}

type NewLocalTokenMetadata struct {
	Label       string
	TokenHash   string
	TokenPrefix string
	TokenLast4  string
	CreatedAt   time.Time
}

type LocalTokenAuthRecord struct {
	ID        int64
	Label     string
	TokenHash string
	Disabled  bool
}

type LocalTokenRepository interface {
	InsertLocalToken(ctx context.Context, meta NewLocalTokenMetadata) (LocalTokenMetadata, error)
	ListLocalTokens(ctx context.Context) ([]LocalTokenMetadata, error)
	DisableLocalToken(ctx context.Context, id int64, disabledAt time.Time) error
	FindLocalTokenByHash(ctx context.Context, hash string) (LocalTokenAuthRecord, error)
}

type LocalTokenManager interface {
	Create(ctx context.Context, label string) (CreatedLocalToken, error)
	List(ctx context.Context) ([]LocalTokenMetadata, error)
	Disable(ctx context.Context, id int64) error
}

type LocalTokenVerifier interface {
	VerifyBearer(ctx context.Context, authorization string) (VerifiedLocalToken, error)
}

var ErrUnauthorized = errors.New("unauthorized")

type Service struct {
	Repo                 LocalTokenRepository
	Now                  func() time.Time
	EphemeralSecretAdded func(string)
}

func GenerateToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "iln_" + base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte("ilonasin-local-client-v1\x00" + token))
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

func (s Service) Create(ctx context.Context, label string) (CreatedLocalToken, error) {
	if label == "" {
		label = "local client"
	}
	token, err := GenerateToken()
	if err != nil {
		return CreatedLocalToken{}, err
	}
	meta, err := s.Repo.InsertLocalToken(ctx, NewLocalTokenMetadata{
		Label:       label,
		TokenHash:   HashToken(token),
		TokenPrefix: Prefix(token),
		TokenLast4:  Last4(token),
		CreatedAt:   s.now(),
	})
	if err != nil {
		return CreatedLocalToken{}, err
	}
	if s.EphemeralSecretAdded != nil {
		s.EphemeralSecretAdded(token)
	}
	return CreatedLocalToken{Token: token, Metadata: meta}, nil
}

func (s Service) List(ctx context.Context) ([]LocalTokenMetadata, error) {
	return s.Repo.ListLocalTokens(ctx)
}

func (s Service) Disable(ctx context.Context, id int64) error {
	return s.Repo.DisableLocalToken(ctx, id, s.now())
}

func (s Service) VerifyBearer(ctx context.Context, authorization string) (VerifiedLocalToken, error) {
	const prefix = "Bearer "
	if !strings.HasPrefix(authorization, prefix) {
		return VerifiedLocalToken{}, ErrUnauthorized
	}
	token := strings.TrimSpace(strings.TrimPrefix(authorization, prefix))
	if !strings.HasPrefix(token, "iln_") {
		return VerifiedLocalToken{}, ErrUnauthorized
	}
	hash := HashToken(token)
	rec, err := s.Repo.FindLocalTokenByHash(ctx, hash)
	if err != nil || rec.Disabled {
		return VerifiedLocalToken{}, ErrUnauthorized
	}
	if subtle.ConstantTimeCompare([]byte(rec.TokenHash), []byte(hash)) != 1 {
		return VerifiedLocalToken{}, ErrUnauthorized
	}
	return VerifiedLocalToken{ID: rec.ID, Label: rec.Label}, nil
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

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
