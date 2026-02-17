package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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

type AuthService struct {
	store     *config.Store
	jwtSecret []byte
}

func NewAuthService(store *config.Store, jwtSecret string) *AuthService {
	return &AuthService{
		store:     store,
		jwtSecret: []byte(jwtSecret),
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

	// Update last used timestamp (fire and forget)
	go s.store.UpdateAPIKeyLastUsed(context.Background(), key.ID)

	return &APIKeyPrincipal{
		KeyID:  key.ID,
		RoleID: key.RoleID,
	}, nil
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
