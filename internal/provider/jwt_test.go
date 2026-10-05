package provider

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
	"time"
)

func TestSignToken(t *testing.T) {
	credentials, _ := newTestCredentials(t)
	issuedAt := time.Unix(1_700_000_000, 0)

	token, err := signToken(credentials, issuedAt, issuedAt.Add(tokenLifetime))
	if err != nil {
		t.Fatalf("signToken failed: %v", err)
	}
	if err := verifyTestToken(token, &credentials.privateKey.PublicKey, credentials.keyID); err != nil {
		t.Fatalf("token does not verify: %v", err)
	}

	parts := strings.Split(token, ".")
	var claims tokenClaims
	if err := decodeSegment(parts[1], &claims); err != nil {
		t.Fatalf("failed to decode claims: %v", err)
	}
	if claims.Issuer != credentials.issuerID {
		t.Errorf("expected iss %q, got %q", credentials.issuerID, claims.Issuer)
	}
	if claims.IssuedAt != issuedAt.Unix() {
		t.Errorf("expected iat %d, got %d", issuedAt.Unix(), claims.IssuedAt)
	}
	if claims.ExpiresAt != issuedAt.Add(tokenLifetime).Unix() {
		t.Errorf("expected exp %d, got %d", issuedAt.Add(tokenLifetime).Unix(), claims.ExpiresAt)
	}
	if claims.ExpiresAt-claims.IssuedAt > int64((20 * time.Minute).Seconds()) {
		t.Errorf("token lifetime exceeds the 20 minute maximum")
	}
}

func TestSignToken_RejectsForeignKey(t *testing.T) {
	credentials, _ := newTestCredentials(t)
	other, _ := newTestCredentials(t)

	token, err := signToken(credentials, time.Now(), time.Now().Add(tokenLifetime))
	if err != nil {
		t.Fatalf("signToken failed: %v", err)
	}
	if err := verifyTestToken(token, &other.privateKey.PublicKey, credentials.keyID); err == nil {
		t.Fatal("expected the token to fail verification against another key")
	}
}

func TestTokenSource_CachesAndRefreshes(t *testing.T) {
	credentials, _ := newTestCredentials(t)
	now := time.Unix(1_700_000_000, 0)
	source := newTokenSource(credentials)
	source.now = func() time.Time { return now }

	first, err := source.Token()
	if err != nil {
		t.Fatalf("Token failed: %v", err)
	}

	now = now.Add(tokenLifetime - tokenRefreshMargin - time.Second)
	cached, err := source.Token()
	if err != nil {
		t.Fatalf("Token failed: %v", err)
	}
	if cached != first {
		t.Fatal("expected the token to be reused before the refresh margin")
	}

	now = now.Add(2 * time.Second)
	refreshed, err := source.Token()
	if err != nil {
		t.Fatalf("Token failed: %v", err)
	}
	if refreshed == first {
		t.Fatal("expected a new token once inside the refresh margin")
	}
}

func TestNewAPICredentials_Validation(t *testing.T) {
	_, validPEM := newTestCredentials(t)

	p384, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(p384)
	if err != nil {
		t.Fatalf("failed to marshal key: %v", err)
	}
	p384PEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))

	cases := []struct {
		name     string
		issuerID string
		keyID    string
		key      string
		want     string
	}{
		{"missing issuer", "", "TESTKEY123", validPEM, "issuer ID is empty"},
		{"missing key ID", "issuer", "", validPEM, "key ID is empty"},
		{"not PEM", "issuer", "TESTKEY123", "garbage", "not PEM encoded"},
		{"not PKCS#8", "issuer", "TESTKEY123", string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("x")})), "PKCS#8"},
		{"wrong curve", "issuer", "TESTKEY123", p384PEM, "P-256"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newAPICredentials(tc.issuerID, tc.keyID, tc.key)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
		})
	}

	if _, err := newAPICredentials("issuer", "TESTKEY123", validPEM); err != nil {
		t.Fatalf("expected a valid key to load, got %v", err)
	}
}
