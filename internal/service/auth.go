package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/faucetdb/faucet/internal/config"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrTokenExpired       = errors.New("token expired")
	ErrKeyRevoked         = errors.New("api key revoked")
)

type APIKeyPrincipal struct {
	KeyID  int64
	RoleID int64
}

type JWTPrincipal struct {
	AdminID int64
	Email   string
}

// LastUsedInterval is the minimum time between two last_used writes for
// the same API key. ValidateAPIKey is on the hot path of every API-key
// request and the config store runs on a single SQLite connection, so
// persisting last_used on every call would compete with the RBAC reads
// each request also needs. It is a variable so tests can shorten it.
var LastUsedInterval = 60 * time.Second

type AuthService struct {
	store     *config.Store
	jwtSecret []byte

	lastUsedMu sync.Mutex
	lastUsed   map[int64]time.Time // key ID -> time of last persisted last_used write
}

func NewAuthService(store *config.Store, jwtSecret string) *AuthService {
	return &AuthService{
		store:     store,
		jwtSecret: []byte(jwtSecret),
		lastUsed:  make(map[int64]time.Time),
	}
}

// ValidateAPIKey checks the provided raw API key against stored key hashes.
func (s *AuthService) ValidateAPIKey(ctx context.Context, rawKey string) (*APIKeyPrincipal, error) {
	hash := hashKey(rawKey)

	key, err := s.store.GetAPIKeyByHash(ctx, hash)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	if !key.IsActive {
		return nil, ErrKeyRevoked
	}

	if key.ExpiresAt != nil && key.ExpiresAt.Before(time.Now()) {
		return nil, ErrTokenExpired
	}

	s.touchLastUsed(key.ID)

	return &APIKeyPrincipal{
		KeyID:  key.ID,
		RoleID: key.RoleID,
	}, nil
}

// touchLastUsed persists the key's last_used timestamp, but at most once
// per LastUsedInterval per key. The write is fire-and-forget: it never
// blocks or fails the request, and a lost write only makes last_used
// slightly stale.
func (s *AuthService) touchLastUsed(keyID int64) {
	now := time.Now()

	s.lastUsedMu.Lock()
	if last, ok := s.lastUsed[keyID]; ok && now.Sub(last) < LastUsedInterval {
		s.lastUsedMu.Unlock()
		return
	}
	s.lastUsed[keyID] = now
	s.lastUsedMu.Unlock()

	go s.store.UpdateAPIKeyLastUsed(context.Background(), keyID) //nolint:errcheck
}

// ValidateJWT verifies a JWT bearer token and returns the associated admin identity.
func (s *AuthService) ValidateJWT(ctx context.Context, tokenStr string) (*JWTPrincipal, error) {
	claims := &jwtClaims{}

	token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	if !token.Valid {
		return nil, ErrInvalidCredentials
	}

	return &JWTPrincipal{
		AdminID: claims.AdminID,
		Email:   claims.Email,
	}, nil
}

// IssueJWT creates a new signed JWT token for the given admin.
func (s *AuthService) IssueJWT(ctx context.Context, adminID int64, email string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := jwtClaims{
		AdminID: adminID,
		Email:   email,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			Issuer:    "faucet",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

type jwtClaims struct {
	AdminID int64  `json:"admin_id"`
	Email   string `json:"email"`
	jwt.RegisteredClaims
}

func hashKey(rawKey string) string {
	h := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(h[:])
}
