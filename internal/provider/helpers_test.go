package provider

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"testing"
)

const (
	testIssuerID = "00000000-0000-0000-0000-000000000000"
	testKeyID    = "TESTKEY123"
)

func newTestCredentials(t *testing.T) (apiCredentials, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate test key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("failed to marshal test key: %v", err)
	}
	privateKeyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	credentials, err := newAPICredentials(testIssuerID, testKeyID, privateKeyPEM)
	if err != nil {
		t.Fatalf("failed to load test credentials: %v", err)
	}
	return credentials, privateKeyPEM
}

func verifyTestToken(token string, publicKey *ecdsa.PublicKey, keyID string) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return fmt.Errorf("expected 3 token segments, got %d", len(parts))
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(signature) != 64 {
		return errors.New("signature is not a 64-byte r||s value")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:])
	if !ecdsa.Verify(publicKey, digest[:], r, s) {
		return errors.New("signature does not verify")
	}
	var header tokenHeader
	if err := decodeSegment(parts[0], &header); err != nil {
		return err
	}
	if header.Algorithm != "ES256" || header.KeyID != keyID || header.Type != "JWT" {
		return fmt.Errorf("unexpected token header %+v", header)
	}
	var claims tokenClaims
	if err := decodeSegment(parts[1], &claims); err != nil {
		return err
	}
	if claims.Audience != tokenAudience {
		return fmt.Errorf("unexpected audience %q", claims.Audience)
	}
	return nil
}

func decodeSegment(segment string, out any) error {
	raw, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
