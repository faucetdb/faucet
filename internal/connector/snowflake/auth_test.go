package snowflake

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTempPEM writes a PEM-encoded private key to a temp file and returns its path.
func writeTempPEM(t *testing.T, blockType string, derBytes []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "key.pem")
	buf := pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: derBytes})
	if err := os.WriteFile(path, buf, 0600); err != nil {
		t.Fatalf("write temp PEM: %v", err)
	}
	return path
}

// generateTestKey creates a 2048-bit RSA key for testing.
func generateTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	return key
}

func TestLoadPrivateKey_PKCS1(t *testing.T) {
	key := generateTestKey(t)
	der := x509.MarshalPKCS1PrivateKey(key)
	path := writeTempPEM(t, "RSA PRIVATE KEY", der)

	loaded, err := loadPrivateKey(path)
	if err != nil {
		t.Fatalf("loadPrivateKey PKCS1: %v", err)
	}
	if loaded.N.Cmp(key.N) != 0 {
		t.Error("loaded key modulus does not match original")
	}
}

func TestLoadPrivateKey_PKCS8(t *testing.T) {
	key := generateTestKey(t)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal PKCS8: %v", err)
	}
	path := writeTempPEM(t, "PRIVATE KEY", der)

	loaded, err := loadPrivateKey(path)
	if err != nil {
		t.Fatalf("loadPrivateKey PKCS8: %v", err)
	}
	if loaded.N.Cmp(key.N) != 0 {
		t.Error("loaded key modulus does not match original")
	}
}

func TestLoadPrivateKey_FileNotFound(t *testing.T) {
	_, err := loadPrivateKey("/nonexistent/path/key.pem")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "read private key file") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoadPrivateKey_InvalidPEM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.pem")
	os.WriteFile(path, []byte("not a pem file"), 0600)

	_, err := loadPrivateKey(path)
	if err == nil {
		t.Fatal("expected error for invalid PEM")
	}
	if !strings.Contains(err.Error(), "no PEM block") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoadPrivateKey_UnsupportedBlockType(t *testing.T) {
	path := writeTempPEM(t, "EC PRIVATE KEY", []byte("fake"))

	_, err := loadPrivateKey(path)
	if err == nil {
		t.Fatal("expected error for unsupported block type")
	}
	if !strings.Contains(err.Error(), "unsupported PEM block type") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBuildJWTDSN(t *testing.T) {
	key := generateTestKey(t)
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	keyPath := writeTempPEM(t, "PRIVATE KEY", der)

	// Use a valid Snowflake DSN format: user@account/db/schema
	dsn := "testuser@testaccount/testdb/PUBLIC?warehouse=WH"

	newDSN, err := buildJWTDSN(dsn, keyPath)
	if err != nil {
		t.Fatalf("buildJWTDSN: %v", err)
	}

	// The rebuilt DSN should contain the authenticator=snowflake_jwt param
	lowerDSN := strings.ToLower(newDSN)
	if !strings.Contains(lowerDSN, "authenticator=snowflake_jwt") {
		t.Errorf("DSN missing authenticator param: %s", newDSN)
	}

	// Should still contain the original user and account info
	if !strings.Contains(newDSN, "testuser") {
		t.Errorf("DSN missing user: %s", newDSN)
	}
}

func TestBuildJWTDSN_InvalidDSN(t *testing.T) {
	key := generateTestKey(t)
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	keyPath := writeTempPEM(t, "PRIVATE KEY", der)

	_, err := buildJWTDSN(":::invalid", keyPath)
	if err == nil {
		t.Fatal("expected error for invalid DSN")
	}
}

func TestBuildJWTDSN_BadKeyFile(t *testing.T) {
	_, err := buildJWTDSN("testuser@testaccount/testdb/PUBLIC", "/nonexistent/key.pem")
	if err == nil {
		t.Fatal("expected error for missing key file")
	}
}
