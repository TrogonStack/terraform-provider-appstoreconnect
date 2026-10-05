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
	"sync"
	"time"
)

const (
	tokenAudience      = "appstoreconnect-v1"
	tokenLifetime      = 15 * time.Minute
	tokenRefreshMargin = 2 * time.Minute
)

type apiCredentials struct {
	issuerID   string
	keyID      string
	privateKey *ecdsa.PrivateKey
}

func newAPICredentials(issuerID, keyID, privateKeyPEM string) (apiCredentials, error) {
	if issuerID == "" {
		return apiCredentials{}, errors.New("issuer ID is empty")
	}
	if keyID == "" {
		return apiCredentials{}, errors.New("key ID is empty")
	}
	key, err := parsePrivateKey(privateKeyPEM)
	if err != nil {
		return apiCredentials{}, err
	}
	return apiCredentials{issuerID: issuerID, keyID: keyID, privateKey: key}, nil
}

func parsePrivateKey(privateKeyPEM string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return nil, errors.New("private key is not PEM encoded")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing PKCS#8 private key: %w", err)
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key must be an ECDSA key, got %T", parsed)
	}
	if key.Curve != elliptic.P256() {
		return nil, errors.New("private key must use the P-256 curve")
	}
	return key, nil
}

type tokenSource struct {
	credentials apiCredentials
	now         func() time.Time

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

func newTokenSource(credentials apiCredentials) *tokenSource {
	return &tokenSource{credentials: credentials, now: time.Now}
}

func (s *tokenSource) Token() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	if s.token != "" && now.Add(tokenRefreshMargin).Before(s.expiresAt) {
		return s.token, nil
	}

	expiresAt := now.Add(tokenLifetime)
	token, err := signToken(s.credentials, now, expiresAt)
	if err != nil {
		return "", err
	}
	s.token = token
	s.expiresAt = expiresAt
	return token, nil
}

type tokenHeader struct {
	Algorithm string `json:"alg"`
	KeyID     string `json:"kid"`
	Type      string `json:"typ"`
}

type tokenClaims struct {
	Issuer    string `json:"iss"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
	Audience  string `json:"aud"`
}

func signToken(credentials apiCredentials, issuedAt, expiresAt time.Time) (string, error) {
	header, err := json.Marshal(tokenHeader{Algorithm: "ES256", KeyID: credentials.keyID, Type: "JWT"})
	if err != nil {
		return "", err
	}
	claims, err := json.Marshal(tokenClaims{
		Issuer:    credentials.issuerID,
		IssuedAt:  issuedAt.Unix(),
		ExpiresAt: expiresAt.Unix(),
		Audience:  tokenAudience,
	})
	if err != nil {
		return "", err
	}

	signingInput := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, credentials.privateKey, digest[:])
	if err != nil {
		return "", fmt.Errorf("signing token: %w", err)
	}

	signature := make([]byte, 64)
	r.FillBytes(signature[:32])
	s.FillBytes(signature[32:])
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}
